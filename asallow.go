package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"gopkg.in/gcfg.v1"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

const PREFIX_URI = "https://stat.ripe.net/data/announced-prefixes/data.json?resource="
const COUNTRY_URI = "https://stat.ripe.net/data/country-resource-list/data.json?resource="
const PREFIX = 0
const COUNTRY = 1

type ipsetInfo struct {
	sync.Mutex
	v4count int
	v6count int
	v4      string
	v6      string
}

type config struct {
	Main struct {
		Allow     []string
		ASN       []string
		Country   []string
		Nocomment bool
	}
}

type resourceInfo struct {
	id  int
	uri string
	cfg []string
}

var ipset ipsetInfo

func readconfig(cfgfile string) config {
	var cfg config
	content, err := ioutil.ReadFile(cfgfile)
	if err != nil {
		log.Fatal(err)
	}
	err = gcfg.ReadStringInto(&cfg, string(content))
	if err != nil {
		log.Fatal("Failed to parse "+cfgfile+":", err)
	}
	return cfg
}

func getURI(uri string) []byte {
	resp, err := http.Get(uri)
	if err != nil {
		log.Fatal("site not available")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("can not read body")
	}
	return body
}

func isIpOrCidr(ipcidr string) *net.IP {
	ip, _, err := net.ParseCIDR(ipcidr)
	if err != nil {
		mystr := strings.Split(ipcidr, "-")
		ip = net.ParseIP(mystr[0])
		if ip == nil {
			return nil
		}
	}
	return &ip
}

func doipset(cfg config) {
	ipset_header := "create AS_allow hash:net family inet comment\n"
	ipset_header += "create AS_allow6 hash:net family inet6 comment\n"
	ipset_header += "create AS_allow_swap hash:net family inet comment\n"
	ipset_header += "create AS_allow_swap6 hash:net family inet6 comment\n"
	ipset_footer := "swap AS_allow AS_allow_swap\n"
	ipset_footer += "swap AS_allow6 AS_allow_swap6\n"
	ipset_footer += "destroy AS_allow_swap\n"
	ipset_footer += "destroy AS_allow_swap6\n"
	ipset_string := ipset_header + ipset.v4 + ipset.v6 + ipset_footer
	if cfg.Main.Nocomment {
		re := regexp.MustCompile(" comment.*")
		ipset_string = re.ReplaceAllString(ipset_string, "")
	}
	cmd := exec.Command("ipset", "-!", "restore")
	cmd.Stdin = strings.NewReader(ipset_string)
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Println("ipset restore failed (see below)")
		log.Fatal(string(out))
	}
}

func parseCountry(mapstring map[string]interface{}) []string {
	var array []string
	datamap := mapstring["data"]
	mapstring = datamap.(map[string]interface{})
	resources := mapstring["resources"]
	mapstring = resources.(map[string]interface{})
	ipv6 := mapstring["ipv6"]
	ipv6_array := ipv6.([]interface{})
	ipv4 := mapstring["ipv4"]
	ipv4_array := ipv4.([]interface{})
	for _, prefix_element := range append(ipv4_array, ipv6_array...) {
		array = append(array, prefix_element.(string))
	}
	return array
}

func parseASN(mapstring map[string]interface{}) []string {
	var array []string
	datamap := mapstring["data"]
	mapstring = datamap.(map[string]interface{})
	prefixes := mapstring["prefixes"]
	prefixes_array := prefixes.([]interface{})
	for _, prefix_element := range prefixes_array {
		mapstring = prefix_element.(map[string]interface{})
		array = append(array, mapstring["prefix"].(string))
	}
	return array
}

func parseBody(body []byte, id int, comment string, sc chan string) {
	var array []string
	var comment_prefix string
	var mapstring map[string]interface{}

	dec := json.NewDecoder(strings.NewReader(string(body)))
	if err := dec.Decode(&mapstring); err != nil {
		log.Fatal(err)
	}
	if id == PREFIX {
		array = parseASN(mapstring)
		comment_prefix = "AS"
	} else {
		array = parseCountry(mapstring)
	}
	for _, prefix := range array {
		ip := isIpOrCidr(prefix) // input validation
		if ip != nil {           // it really is an IP
			if ip.To4() != nil { // is it IPv4
				ipset.Lock()
				ipset.v4 += "add AS_allow_swap " + prefix + " comment " + comment_prefix + comment + "\n"
				ipset.v4count += 1
				ipset.Unlock()
			} else { // ipv6
				ipset.Lock()
				ipset.v6 += "add AS_allow_swap6 " + prefix + " comment " + comment_prefix + comment + "\n"
				ipset.v6count += 1
				ipset.Unlock()
			}
		} else {
			log.Println("not an ip (range): " + prefix + comment_prefix + " " + comment)
		}
	}
	//fmt.Println("starting thread for: "+comment_prefix)
	sc <- "done"
}

func addAllowed(allowed []string) {
	for _, el := range allowed {
		ip := isIpOrCidr(el)
		if ip != nil { //really an IP
			if ip.To4() != nil {
				ipset.v4 += "add AS_allow_swap " + el + " comment \"read from asallow.conf\"\n"
				ipset.v4count += 1
			} else {
				ipset.v6 += "add AS_allow_swap6 " + el + " comment \"read from asallow.conf\"\n"
				ipset.v6count += 1
			}
		} else {
			log.Println("not an ip (range): " + el)
		}
	}
}

func main() {
	if os.Geteuid() != 0 {
		log.Fatal("This needs to be run as root")
	}
	runtime.GOMAXPROCS(runtime.NumCPU())
	cfgfile := flag.String("conf", "asallow.conf", "a valid config file")
	flag.Parse()

	counter := 0
	sc := make(chan string)

	// parse the config
	cfg := readconfig(*cfgfile)
	resources := []resourceInfo{
		{PREFIX, PREFIX_URI, cfg.Main.ASN},
		{COUNTRY, COUNTRY_URI, cfg.Main.Country},
	}

	// add always the static entries
	addAllowed(cfg.Main.Allow)
	doipset(cfg)

	for _, resource := range resources {
		for i, uri_id := range resource.cfg {
			go func(uri_id string, resource resourceInfo) {
				if counter > 0 && i%2 == 0 { // max 2 rqs
					time.Sleep(time.Second)
				}
				counter += 1
				body := getURI(resource.uri + uri_id)
				go parseBody(body, resource.id, uri_id, sc)
			}(uri_id, resource)
		}
	}

	for range append(cfg.Main.Country, cfg.Main.ASN...) {
		<-sc
	}

	doipset(cfg)

	fmt.Printf("%v ipv4 / %v ipv6 subnets added\n", ipset.v4count, ipset.v6count)
	fmt.Println("AS_allow and AS_allow6 ipset created/modified")
}
