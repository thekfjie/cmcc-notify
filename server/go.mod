module github.com/thekfjie/cmcc-notify/server

go 1.25.0

require (
	github.com/thekfjie/cmcc-notify/cmcc v0.0.0
	gopkg.in/yaml.v3 v3.0.1
)

require github.com/gorilla/websocket v1.5.3 // indirect

replace github.com/thekfjie/cmcc-notify/cmcc => ../cmcc
