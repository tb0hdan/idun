#!/bin/bash

#
# {"domains":["example.com", "example.org"]}
#
$(dirname $0)/idun -debug -webserver-port 0 -custom-domains-url http://localhost:8000/domains
