module github.com/lanos/lanos/mobile

go 1.22

require github.com/lanos/lanos/core v0.0.0

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/flynn/noise v1.1.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grandcat/zeroconf v1.0.0 // indirect
	github.com/miekg/dns v1.1.27 // indirect
	golang.org/x/crypto v0.28.0 // indirect
	golang.org/x/net v0.30.0 // indirect
	golang.org/x/sys v0.26.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace github.com/lanos/lanos/core => ../core
