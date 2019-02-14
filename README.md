## Firefly - Go Ethereum 2.0

This repository is an experimental work towards a Go implementation of the Ethereum 2.0 protocol. The rationale behind the separate repo is to permit cowboy coding, prototyping and experimenting without the strict security requirements of the Geth codebase. The eventual end goal is to merge this work upstream into [ethereum/go-ethereum](https://github.com/ethereum/go-ethereum) once it's stable (whenever that might happen). Until then, consider any code in this repository unstable, unworkable, unsecure, unreliable and any other un-bad-thing.

## Contributing

Although there are no strict restrictions on who merges what and when (for now), we should strive towards a clean commit history and developer workflow, mirroring that of go-ethereum:

 * Don't work on the master branch. Create your own feature branch in your own fork and open a pull request upstream. Feel free to merge it yourself. This keeps the repository clean and people don't mess with each others' history.
 * Have proper commit messages (`package1, package2: commit subject`) and squash when needed. Although this is an experimental repository we should still keep proper developer practices and make tracking commits meaningful.
 * Don't break master, others may depend on it. If you broke it, no problem, fix it (just don't leave it borked).

## License

Code contributed into this repository is licensed under BSD-3, but some upstream go-ethereum dependencies might be LGPL. Long term our goal is to fully sanitize the license, but we don't have immediate plans to relicense go-ethereum until it's clear which parts are actually needed in Ethereum 2.0.
