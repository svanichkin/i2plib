# i2plib

Go library for working with I2P via the SAM v3.1 API.

**What’s included**
- SAM message helpers and I2P base64/base32 (`sam.go`)
- SAM client primitives (HELLO, DEST GENERATE, SESSION CREATE, NAMING LOOKUP, STREAM CONNECT/ACCEPT) (`aiosam.go`)
- Stream tunnels (client/server) (`tunnel.go`)

## Install

```sh
go get github.com/svanichkin/i2plib
```

## Configuration

The default SAM address is `127.0.0.1:7656`.

You can override it via:

```sh
export I2P_SAM_ADDRESS=127.0.0.1:7656
```

## Usage

### Create destination + session

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/svanichkin/i2plib"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	samAddr := i2plib.GetSAMAddress()
	sess, err := i2plib.CreateSession(ctx, "test", samAddr, "STREAM", i2plib.DefaultSigType, nil, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer sess.Close()

	log.Printf("session=%s dest=%s.b32.i2p", sess.Name(), sess.Destination.Base32())
}
```

### Client tunnel (local TCP -> remote I2P destination)

```go
samAddr := i2plib.GetSAMAddress()
samClient := i2plib.NewDefaultSAMClient(samAddr)

tun := i2plib.NewClientTunnel(
	i2plib.Address{Host: "127.0.0.1", Port: 6668},
	"irc.echelon.i2p",
	samAddr,
	samClient,
	nil,    // destination (nil => create new)
	"",     // session name ("" => auto)
	nil,    // SAM options
)

ctx := context.Background()
if err := tun.Run(ctx); err != nil {
	log.Fatal(err)
}
defer tun.Stop()
```

### Server tunnel (accept from I2P -> local TCP)

```go
samAddr := i2plib.GetSAMAddress()
samClient := i2plib.NewDefaultSAMClient(samAddr)

tun := i2plib.NewServerTunnel(
	i2plib.Address{Host: "127.0.0.1", Port: 8080},
	samAddr,
	samClient,
	nil, // destination (nil => transient)
	"",
	nil,
)

ctx := context.Background()
if err := tun.Run(ctx); err != nil {
	log.Fatal(err)
}
defer tun.Stop()
```

## Tests

```sh
GOCACHE="$PWD/.gocache" go test ./...
```

`tunnel_mock_sam_test.go` includes a mock SAM server. In restricted sandboxes that disallow `listen(2)`, these tests will be skipped.
