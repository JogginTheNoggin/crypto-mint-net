module github.com/JogginTheNoggin/crypto-mint-net/gui

go 1.27.0

toolchain go1.27.3

require github.com/JogginTheNoggin/crypto-mint-net v0.0.0

require (
	github.com/btcsuite/btcd/btcec/v2 v2.5.1 // indirect
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0 // indirect
	github.com/cloudflare/circl v1.6.3 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.1 // indirect
	github.com/ebitengine/purego v0.11.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.12 // indirect
	github.com/zeebo/blake3 v0.2.4 // indirect
	golang.org/x/crypto v0.58.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
)

// The GUI always builds on the headless source in this repository. If the
// GUI moves to its own repository, this becomes a version requirement on
// the core module (docs/gui.md).
replace github.com/JogginTheNoggin/crypto-mint-net => ../
