# asallow

## What
Asallow creates 2 ipsets (AS_allow and AS_allow6), and populates them based on data from the RIPEstat Data API
(https://stat.ripe.net/index/documentation/interfaces-apis) (and optionally your own manually specified subnets)

## Why
An 'easy way' to whitelist providers and/or countries.  
E.g. to only allow ssh access from specific countries.

## Requirements
* Linux kernel >= 2.6.32 supporting ipset (net:hash) (only tested on 3.17.x)
* ipset 6.20+
* root/sudo access
* iptables

## Config
By default it searches for asallow.conf in the current directory, you can also explicitly specify a config with -conf switch.  
It will not run without a config file.

```
 ./asallow -h
Usage of ./asallow:
  -conf="asallow.conf": a valid config file
```
Config file as a [main] section and multivalue allow, ASN or country statements. Any combination works.

* allow=range|ip

  Specify an IPv4 or IPv6 ip address or CIDR range.  
  This will always be added to the ipset AS_allow or AS_allow6 (for IPv6)

* ASN=AS number

  Specify an AS number.  
  Ranges (currently) announced by this AS will be added to the ipset AS_allow or AS_allow6 (for IPv6) (AS number: see http://en.wikipedia.org/wiki/Autonomous_System_(Internet))

* country=country code

  2-digit ISO-3166 country code.  
  Ranges (currently) associated with the selected country will be added to the ipset AS_allow or AS_allow6 (for IPv6)

### Example configuration
```
[main]
#ip ranges or ip addresses which should always be add
allow=10.0.0.0/8
allow=192.168.0.0/16
allow=fe80::/10

#prefixes of providers which should be looked up dynamically (using ripestat)
ASN=2611 #belnet
ASN=6848 #telenet
#ASN=5432 #mobistar

#allow a whole country
country=be
#country=nl
#country=fr
```

## Running it
```
# ./asallow
760 ipv4 / 169 ipv6 subnets added
AS_allow and AS_allow6 ipset created/modified
```

Check the contents of the sets with ipset:

```
# ipset list AS_allow
Name: AS_allow
Type: hash:net
Revision: 5
Header: family inet hashsize 1024 maxelem 65536 comment
Size in memory: 47488
References: 2
Members:
217.72.224.0/20 comment "be"
86.39.128.0/17 comment "be"
193.33.52.0/23 comment "be"
194.42.208.0/24 comment "be"
80.66.128.0/20 comment "be"
193.58.40.0/24 comment "AS6848"
213.246.192.0/18 comment "be"
....
```

## Integration with IPtables
Simple example:
Only allow ssh from AS_allow ranges and drop the rest.

```
iptables -A INPUT -p tcp -m tcp --dport 22 -m set --match-set AS_allow src -j ACCEPT
iptables -A INPUT -p tcp -m tcp --dport 22 -j REJECT
```
