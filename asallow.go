package main

import (
	"code.google.com/p/gcfg"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os/exec"
	"strings"
)

const PREFIX_URI="https://stat.ripe.net/data/announced-prefixes/data.json?resource="

var ipset_count int = 0

func readconfig(cfgfile string) []string {
	data, err := ioutil.ReadFile(cfgfile)
    if err != nil {
        log.Fatal(err)
    }
	cfgStr := string(data)
	cfg := struct {
		Main struct {
			Allow []string
		}
	}{}
	err = gcfg.ReadStringInto(&cfg, cfgStr)
	if err != nil {
		log.Fatal("Failed to parse "+ cfgfile +":",err)
	}
	return cfg.Main.Allow
}

func getAS(ASnumber string) []byte  {
	resp, err := http.Get(PREFIX_URI + ASnumber)
	if err != nil {
		log.Fatal("site not available")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("can not read body")
	}
    return body
}

func doipset(ipset_string string) {
	cmd := exec.Command("ipset", "-!", "create", "AS_allow", "hash:net", "comment")
	err := cmd.Run()
    if err != nil {
        log.Fatal(err)
    }
    cmd = exec.Command("ipset","-!","create","AS_allow_swap","hash:net", "comment")
    cmd.Run()
    if err != nil {
        log.Fatal(err)
    }
	cmd = exec.Command("ipset", "-!", "restore")
	cmd.Stdin = strings.NewReader(ipset_string)
	err = cmd.Run()
	if err != nil {
		log.Fatal("ip addresses could not be added", err)
	}
    cmd = exec.Command("ipset","swap","AS_allow","AS_allow_swap")
    cmd.Run()
    if err != nil {
        log.Fatal(err)
    }
    cmd = exec.Command("ipset","destroy","AS_allow_swap")
    cmd.Run()
}

func parseBody(body []byte, ASnumber string) string {
    ipset_string := ""
	dec := json.NewDecoder(strings.NewReader(string(body)))
	var mapstring map[string]interface{}
	if err := dec.Decode(&mapstring); err != nil {
		log.Fatal(err)
	}
	datamap := mapstring["data"]
	mapstring = datamap.(map[string]interface{})
	prefixes := mapstring["prefixes"]
	prefixes_array := prefixes.([]interface{})
	for _, prefix_element := range prefixes_array {
		mapstring = prefix_element.(map[string]interface{})
		if strings.Contains(mapstring["prefix"].(string), "::") != true {
			ipset_string += "add AS_allow_swap " + mapstring["prefix"].(string) + " comment AS" + ASnumber + "\n"
			ipset_count += 1
		}
	}
    return ipset_string
}

func addAllowed(ipset_string string,allowed []string) string {
	for _, el := range allowed {
		ipset_string += "add AS_allow_swap " + el + " comment \"read from asallow.conf\"\n"
	}
    return ipset_string
}

func main() {
	ASnumber := flag.String("ASN", "42", "an valid AS number")
    cfgfile := flag.String("conf","asallow.conf","a valid config file")
	flag.Parse()

	allowed := readconfig(*cfgfile)
    body := getAS(*ASnumber)
    ipset_string := parseBody(body,*ASnumber)
    ipset_string = addAllowed(ipset_string,allowed)
    doipset(ipset_string)

	fmt.Printf("%v ip addresses added\n", ipset_count)
}
