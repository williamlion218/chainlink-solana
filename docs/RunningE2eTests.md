# Running tests

## Installation
`make build && make install`


## Configuration
The main test config logic resides in the `integration-tests/testconfig/` directory. Everything is configured using TOML. The minimum OCR2 required values can be located at `integration-tests/testconfig/default.toml`, these values default to running the tests locally in docker using devnet.

### Combinations
There are a few possibile combinations to run tests that we support.

**Devnet** 
Devnet requires previously deployed programs that are owned by the person running the tests. The program ID's are required for testnet, but ignored in localnet.

- `Common.network` needs to be set to `devnet` which will instruct the tests to run against devnet
- `ocr2_program_id`, `access_controller_program_id`, `store_program_id`, `link_token_address`, `vault_address` need to be set so the tests know what programs to use so we avoid deploying each time.
- `E2E_TEST_COMMON_RPC_URL` and `E2E_TEST_COMMON_WS_URL` need to be set

**Localnet**
Setting localnet will instruct the tests to run in localnet, the program ID's are not taken from the TOML in this scenario, but rather defined in the `integration-tests/config/config.go`.

**K8s**

Running in Kubernetes will require aws auth.

- `Common.inside_k8` needs to be set to true if you want to run the tests in k8

### Overrides

By default all values are pulled either from `default.toml` or if we create an `overrides.toml` where we want to set new values or override existing values. Both `default.toml` and `overrides.toml` will end up being merged where values that are set in both files will be taken based on the value in `overrides.toml`.

## Run tests

`cd integration-tests/smoke && go test -timeout 24h -count=1 -run TestSolanaOCRV2Smoke -test.timeout 30m;`

### On demand soak test

Navigate to the [workflow](https://github.com/smartcontractkit/chainlink-solana/actions/workflows/soak.yml). The workflow takes in 3 parameters:

- Base64 string of the .toml configuration
- Core image tag which defaults to develop
- Test runner tag, only tag needs to be supplied
    - In order to create the test image a label needs to be added to the PR `build-test-image` which will start building it on every push.

Create an `overrides.toml` file in `integration-tests/testconfig` and run `cat overrides.toml | base64`. `inside_k8` needs to be set to true in the .toml in order to run the tests in kubernetes.

#### Local

If you want to kick off the test from local:

- `export TEST_SUITE: soak`
- `export DETACH_RUNNER: true`
- `export ENV_JOB_IMAGE: <internal_repo>/chainlink-solana-tests:<tag>`
- Base64 the .toml config
- Run `export BASE64_CONFIG_OVERRIDE="<config>"`
- cd integration-tests/soak && go test -timeout 24h -count=1 -run TestSolanaOCRV2Soak -test.timeout 30m;
