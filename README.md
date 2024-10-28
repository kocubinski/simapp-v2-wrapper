# cosmossdk.io/simapp/v2 wrapper

This small go module shows how one might wrap simapp/v2 for testing with an alternate consensus componment.

The line below in `go.work` should be edited to point to the location of simapp/v2 on your local machine.

```golang
use (
    .
    ../cosmos-sdk/sdk-dev/simapp/v2
)
```

## Usage

```bash
go build -o notsimd main.go
SIMD_BIN=./notsimd ../cosmos-sdk/sdk-dev/scripts/init-simapp-v2.sh
./notsimd start
```
