# Changelog

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
