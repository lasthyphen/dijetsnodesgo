<div align="center">
  <img src="resources/DijetsNode.png?raw=true">
</div>

---

A Golang Node implementation for Dijets network.

## Installation

Dijets is an incredibly lightweight protocol, so the minimum computer requirements are quite modest.
Note that as network usage increases, hardware requirements may change.

The minimum recommended hardware specification for nodes connected to Mainnet is:

- CPU: Equivalent of 8 AWS vCPU
- RAM: 16 GiB
- Storage: 1 TiB
- OS: Ubuntu 20.04/22.04 or macOS >= 12
- Network: Reliable IPv4 or IPv6 network connection, with an open public port.

If you plan to build DijetsNodeGo from source, you will also need the following software:

- [Go](https://golang.org/doc/install) version >= 1.18.1
- [gcc](https://gcc.gnu.org/)
- g++

### Native Install

Clone the DijetsNodeGo repository:

```sh
git clone git@github.com:lasthyphen/dijetsnodesgo.git
cd dijetsnodesgo
```

This will clone and checkout to `master` branch.

#### Building the Dijets Executable

Build Dijets by running the build script:

```sh
./scripts/build.sh
```

The output of the script will be the Dijets binary named `dijetsnodesgo`. It is located in the build directory:

```sh
./build/dijetsnodesgo
```

### Binary Install

Download the [latest build](https://github.com/lasthyphen/dijetsnodesgo/releases/latest) for your operating system and architecture.

The Dijets binary to be executed is named `dijetsnodesgo`.

### Docker Install

Make sure docker is installed on the machine - so commands like `docker run` etc. are available.

Building the docker image of latest dijetsnodesgo branch can be done by running:

```sh
./scripts/build_image.sh
```

To check the built image, run:

```sh
docker image ls
```

The image should be tagged as `hyphenesc/dijetsnodesgo:xxxxxxxx`, where `xxxxxxxx` is the shortened commit of the Dijets source it was built from. To run the avalanche node, run:

```sh
docker run -ti -p 9650:9650 -p 9651:9651 hyphenesc/dijetsnodesgo:xxxxxxxx /dijetsnodesgo/build/dijetsnodesgo
```

## Running Dijets

### Connecting to Mainnet

To connect to the Dijets Mainnet, run:

```sh
./build/dijetsnodesgo
```

You should see some pretty ASCII art and log messages.

You can use `Ctrl+C` to kill the node.

### Connecting to Fuji

To connect to the Fuji Testnet, run:

```sh
./build/dijetsnodesgo --network-id=fuji
```

## Bootstrapping

A node needs to catch up to the latest network state before it can participate in consensus and serve API calls. This process, called bootstrapping, currently takes several days for a new node connected to Mainnet.

A node will not [report healthy](https://docs.djtx.network/build/dijetsnodesgo-apis/health) until it is done bootstrapping.

Improvements that reduce the amount of time it takes to bootstrap are under development.

The bottleneck during bootstrapping is typically database IO. Using a more powerful CPU or increasing the database IOPS on the computer running a node will decrease the amount of time bootstrapping takes.

## Generating Code

Avalanchego uses multiple tools to generate efficient and boilerplate code.

### Running protobuf codegen

To regenerate the protobuf go code, run `scripts/protobuf_codegen.sh` from the root of the repo.

This should only be necessary when upgrading protobuf versions or modifying .proto definition files.

To use this script, you must have [buf](https://docs.buf.build/installation) (v1.9.0), protoc-gen-go (v1.28.0) and protoc-gen-go-grpc (v1.2.0) installed.

To install the buf dependencies:

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.28.0
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.2.0
```

If you have not already, you may need to add `$GOPATH/bin` to your `$PATH`:

```sh
export PATH="$PATH:$(go env GOPATH)/bin"
```

If you extract buf to ~/software/buf/bin, the following should work:

```sh
export PATH=$PATH:~/software/buf/bin/:~/go/bin
go get google.golang.org/protobuf/cmd/protoc-gen-go
go get google.golang.org/protobuf/cmd/protoc-gen-go-grpc
scripts/protobuf_codegen.sh
```

For more information, refer to the [GRPC Golang Quick Start Guide](https://grpc.io/docs/languages/go/quickstart/).

### Running protobuf codegen from docker

```sh
docker build -t dijets:protobuf_codegen -f api/Dockerfile.buf .
docker run -t -i -v $(pwd):/opt/dijets -w/opt/dijets dijets:protobuf_codegen bash -c "scripts/protobuf_codegen.sh"
```

### Running mock codegen

To regenerate the [gomock](https://github.com/golang/mock) code, run `scripts/mock.gen.sh` from the root of the repo.

This should only be necessary when modifying exported interfaces or after modifying `scripts/mock.mockgen.txt`.

## Versioning

### Library Compatibility Guarantees

Because DijetsNodeGo's version denotes the network version, it is expected that interfaces exported by DijetsNodeGo's packages may change in `Patch` version updates.

### API Compatibility Guarantees

APIs exposed when running DijetsNodeGo will maintain backwards compatibility, unless the functionality is explicitly deprecated and announced when removed.

## Supported Platforms

DijetsNodeGo can run on different platforms, with different support tiers:

- **Tier 1**: Fully supported by the maintainers, guaranteed to pass all tests including e2e and stress tests.
- **Tier 2**: Passes all unit and integration tests but not necessarily e2e tests.
- **Tier 3**: Builds but lightly tested (or not), considered _experimental_.
- **Not supported**: May not build and not tested, considered _unsafe_. To be supported in the future.

The following table lists currently supported platforms and their corresponding
DijetsNodeGo support tiers:

| Architecture | Operating system | Support tier  |
| :----------: | :--------------: | :-----------: |
|    amd64     |      Linux       |       1       |
|    arm64     |      Linux       |       2       |
|    amd64     |      Darwin      |       2       |
|    amd64     |     Windows      |       3       |
|     arm      |      Linux       | Not supported |
|     i386     |      Linux       | Not supported |
|    arm64     |      Darwin      | Not supported |

To officially support a new platform, one must satisfy the following requirements:

| DijetsNodeGo continuous integration | Tier 1  | Tier 2  | Tier 3  |
| ---------------------------------- | :-----: | :-----: | :-----: |
| Build passes                       | &check; | &check; | &check; |
| Unit and integration tests pass    | &check; | &check; |         |
| End-to-end and stress tests pass   | &check; |         |         |

DijetsNodeGo is a tweaked fork of AvalancheGo which maintains upstream changes.

## Bugs / Vulnerabilities
--

### Versioning

DijetsNodeGo is first and foremost a client for the Dijets network. The versioning of DijetsNodeGo follows that of the Dijets network.

- `v0.x.x` indicates a development network version.
- `v1.x.x` indicates a production network version.
- `vx.[Upgrade].x` indicates the number of network upgrades that have occurred.
- `vx.x.[Patch]` indicates the number of client upgrades that have occurred since the last network upgrade.