# Changelog

## [0.4.0](https://github.com/fguimond/kairos/compare/v0.3.1...v0.4.0) (2026-06-16)


### Features

* add standalone Go client library ([#56](https://github.com/fguimond/kairos/issues/56)) ([856b125](https://github.com/fguimond/kairos/commit/856b125bf5cf048a31e868793f35e0b9b3bc79e6))


### Miscellaneous Chores

* bump all Go dependencies ([#64](https://github.com/fguimond/kairos/issues/64)) ([7eb87da](https://github.com/fguimond/kairos/commit/7eb87daafa3e85b3c818ee571b4735055d1c9b6c))
* bump runtime base image to alpine:3.24 ([#58](https://github.com/fguimond/kairos/issues/58)) ([505dc50](https://github.com/fguimond/kairos/commit/505dc5004b23f495d564c95bbbe3f35b01c2e256))
* **deps:** update rabbitmq docker tag to v4 ([#62](https://github.com/fguimond/kairos/issues/62)) ([2aa0743](https://github.com/fguimond/kairos/commit/2aa074348b9364759bf917c7889fd1b6c9adf750))
* enable Renovate for dependency updates ([#60](https://github.com/fguimond/kairos/issues/60)) ([0076963](https://github.com/fguimond/kairos/commit/0076963402b06000fa1a8a344d8d912ce0d366e3))


### Continuous Integration

* include chores and ci commits in release-please changelog ([#65](https://github.com/fguimond/kairos/issues/65)) ([aa80f9e](https://github.com/fguimond/kairos/commit/aa80f9e94b2261ae03eed345f2cc2ffb83fd1985))

## [0.3.1](https://github.com/fguimond/kairos/compare/v0.3.0...v0.3.1) (2026-06-11)


### Bug Fixes

* publish Docker image from release-please workflow ([#54](https://github.com/fguimond/kairos/issues/54)) ([bdb39cf](https://github.com/fguimond/kairos/commit/bdb39cf8493ede8e3c195ab97cc37078572b84ed))

## [0.3.0](https://github.com/fguimond/kairos/compare/v0.2.0...v0.3.0) (2026-06-10)


### Features

* add filtering support to GET /api/v1/schedules ([#41](https://github.com/fguimond/kairos/issues/41)) ([4922af5](https://github.com/fguimond/kairos/commit/4922af503d4d82761320d45289ac553674587768))
* add pagination support to GET /api/v1/schedules ([d220154](https://github.com/fguimond/kairos/commit/d220154862df072785cb048fff8d416f00d1e6cf))
* add sorting support to GET /api/v1/schedules ([#43](https://github.com/fguimond/kairos/issues/43)) ([8181729](https://github.com/fguimond/kairos/commit/81817297f4961adec9a9dee54ed1edf54873dd92))


### Bug Fixes

* null out next_run_at for non-active schedules ([#48](https://github.com/fguimond/kairos/issues/48)) ([937875b](https://github.com/fguimond/kairos/commit/937875b2cb04ae66bce9120673a768670d5e113c))

## [0.2.0](https://github.com/fguimond/kairos/compare/v0.1.0...v0.2.0) (2026-06-04)


### Features

* add API key authentication middleware ([#27](https://github.com/fguimond/kairos/issues/27)) ([6a481e0](https://github.com/fguimond/kairos/commit/6a481e02cde0fb1cad2a372a0c7d38a6364a0225))
* add CLI flags as alternative to environment variables ([#32](https://github.com/fguimond/kairos/issues/32)) ([42c32a6](https://github.com/fguimond/kairos/commit/42c32a686e1a9b082669ce3a24af793c7fd9e7eb))
* prefix all environment variables with KAIROS_ ([#30](https://github.com/fguimond/kairos/issues/30)) ([007065e](https://github.com/fguimond/kairos/commit/007065e1d3b344217d2c5d779c607b9293391db7))
* restructure CLI with cobra/viper subcommands ([#34](https://github.com/fguimond/kairos/issues/34)) ([2723346](https://github.com/fguimond/kairos/commit/2723346cd7a43738ce043b39236aefd5830b965c))

## [0.1.0](https://github.com/fguimond/kairos/compare/v0.0.0...v0.1.0) (2026-06-03)


### Features

* add timezone support for cron schedules ([#11](https://github.com/fguimond/kairos/issues/11)) ([4b06280](https://github.com/fguimond/kairos/commit/4b06280f722758111380df01709778839e946acf))
* add transaction management to store and service layers ([#20](https://github.com/fguimond/kairos/issues/20)) ([5874cf2](https://github.com/fguimond/kairos/commit/5874cf2b6ac9475959cd7f1028c010d1f1ecb9f1))
