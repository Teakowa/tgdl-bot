# Changelog

## [1.2.2](https://github.com/Teakowa/tgdl-bot/compare/v1.2.1...v1.2.2) (2026-03-22)


### Bug Fixes

* **downloader:** recover stale running tasks ([ec870a6](https://github.com/Teakowa/tgdl-bot/commit/ec870a6304f6aaf8eb14df6a2febdae8b5f79bec))
* **release:** update GitHub token reference in release workflow ([d38d0b6](https://github.com/Teakowa/tgdl-bot/commit/d38d0b6c6374ba08b67359dfd12609f7cdd259d6))

## [1.2.1](https://github.com/Teakowa/tgdl-bot/compare/v1.2.0...v1.2.1) (2026-03-22)


### Bug Fixes

* **storage:** support D1 add-column migrations ([01832fb](https://github.com/Teakowa/tgdl-bot/commit/01832fbb7e71d31a0dccfdd5b98137f79d4f8e00))

## [1.2.0](https://github.com/Teakowa/tgdl-bot/compare/v1.1.0...v1.2.0) (2026-03-22)


### Features

* add config and entrypoint skeletons ([cfb076b](https://github.com/Teakowa/tgdl-bot/commit/cfb076b33908d4db400699b1749f336e7c72e811))
* **bot:** add explicit forward target support ([39e4392](https://github.com/Teakowa/tgdl-bot/commit/39e43926c9a65b2d20e586cce106cc9b58ee5bed))
* **bot:** add queue action menus ([2754eb8](https://github.com/Teakowa/tgdl-bot/commit/2754eb886aae77f05892187567e986f006aa82ec))
* **bot:** add queue/delete/retry commands with inline callbacks ([21c1c50](https://github.com/Teakowa/tgdl-bot/commit/21c1c503fc5487c8a2555b1235cc0f7e0fece92e))
* **bot:** add webhook mode with polling fallback ([e6bda4f](https://github.com/Teakowa/tgdl-bot/commit/e6bda4f75fbb7f22e616a79e504fa3732495795d))
* **bot:** rebuild failed duplicate tasks and add reaction feedback ([cc9ed1d](https://github.com/Teakowa/tgdl-bot/commit/cc9ed1da15fcc7dc1a4935d8576225f79a98d930))
* **bot:** support force delete for non-running tasks ([cf63e65](https://github.com/Teakowa/tgdl-bot/commit/cf63e654bb9d083255720fb5822ffb8049460bc9))
* **deploy:** add single-image docker runtime with embedded tdl ([6a126d9](https://github.com/Teakowa/tgdl-bot/commit/6a126d97a67ee51ce3d188a9717479b39fde678e))
* implement phase 1 task pipeline and downloader consumer ([141eace](https://github.com/Teakowa/tgdl-bot/commit/141eaceea45d6baa3a2bb1dc52d48ccf39577749))
* **observability:** add bot/downloader lifecycle logs for compose ([3a61bbc](https://github.com/Teakowa/tgdl-bot/commit/3a61bbc22db88a0efc496d88364d7d0b4465f5d4))
* **phase2:** switch task model from download to forwarding ([31c58cd](https://github.com/Teakowa/tgdl-bot/commit/31c58cdb8692ae241cb34888b2dd8c7657ff8c04))
* retry failed tasks on startup and quote bot replies ([41ec61b](https://github.com/Teakowa/tgdl-bot/commit/41ec61b9d85dd7090bac4343198cc8efa02c539e))
* **scaffold:** initialize phase1 tgdl bot project skeleton ([6758814](https://github.com/Teakowa/tgdl-bot/commit/67588147da9f0c6bf43d02f587053b4289a5b357))
* **storage:** migrate task persistence to D1 and split deployment compose ([5bc2461](https://github.com/Teakowa/tgdl-bot/commit/5bc2461ebf5da287d1d0bd5eb2924b54ae95f802))
* **sync:** move task status updates to bot via status queue ([e85e44f](https://github.com/Teakowa/tgdl-bot/commit/e85e44f593df99455697e41aba0c5969f4227e99))


### Bug Fixes

* **bot:** add button selection to delete command ([0d7a50f](https://github.com/Teakowa/tgdl-bot/commit/0d7a50fbbc9da68b04dd8fd99e684ad4e259fc75))
* **bot:** skip redundant initial status sync ([8053fd1](https://github.com/Teakowa/tgdl-bot/commit/8053fd1c7a18756935aacd45a865f437ff0ffa33))
* **bot:** use Telegram-compatible reaction emojis ([30c6690](https://github.com/Teakowa/tgdl-bot/commit/30c6690c6b75cc7278336709232a69d151aa1b75))
* **downloader:** collapse byte-based tdl progress logs ([d306ebe](https://github.com/Teakowa/tgdl-bot/commit/d306ebe67085a2bd24c34904534747c128ac6ff0))
* **downloader:** enforce single active tdl executor ([c8081fc](https://github.com/Teakowa/tgdl-bot/commit/c8081fc3e008b9abedfeaa44db08f8e96a33f435))
* **downloader:** reduce tdl progress stream log noise ([a0eb83c](https://github.com/Teakowa/tgdl-bot/commit/a0eb83ce6376453fd039636198f37d81ef763ecb))
* **downloader:** refresh task before final status notify ([b5db396](https://github.com/Teakowa/tgdl-bot/commit/b5db3964648e103e90591a5c123caaf59d5109a9))
* **downloader:** retry transient tdl interruptions and enable reconnect ([f3b784b](https://github.com/Teakowa/tgdl-bot/commit/f3b784b3c44e068f6f86a585978c8baa00154ce5))
* **downloader:** stream tdl stdout stderr logs ([546bd95](https://github.com/Teakowa/tgdl-bot/commit/546bd957f5c1c2c02d229cf563748566cadd9bbb))
* **downloader:** switch tdl forward to --from and tighten cli error classification ([ce9f002](https://github.com/Teakowa/tgdl-bot/commit/ce9f00230a8be7e152a016b60d53aa311b9e085f))
* **forward:** make target chat optional for saved messages default ([18b7e25](https://github.com/Teakowa/tgdl-bot/commit/18b7e25d87cfa12f7ac187b3bfab673e3d6e3e51))
* implement real tdl preflight and stabilize sqlite startup ([761fdd6](https://github.com/Teakowa/tgdl-bot/commit/761fdd6236b195d90eb9ecf03c95abecb0ec5039))
* parse Telegram message date as unix timestamp ([898f552](https://github.com/Teakowa/tgdl-bot/commit/898f552eca814eafe75a4c4c2314c4977d91031b))
* **queue:** align pull and ack/retry with Cloudflare contracts ([16c169d](https://github.com/Teakowa/tgdl-bot/commit/16c169d6cf7a82c60bd15822812d5faa97a75b67))
* **queue:** use single-message push contract for enqueue ([53a82e4](https://github.com/Teakowa/tgdl-bot/commit/53a82e4ed109f3e2153e0005081bfaffbf7fabd6))
* **queue:** wrap push messages with body for Cloudflare API ([5a25ef5](https://github.com/Teakowa/tgdl-bot/commit/5a25ef5a06b66a19be9e274ba48b255ff7ff2f2d))
* **reaction:** preserve final status when refreshing message ids ([794c3c9](https://github.com/Teakowa/tgdl-bot/commit/794c3c995e716ae2c62a7241d9d4eef40130d963))
* **reaction:** use telegram-compatible emojis for source message ([c78a73a](https://github.com/Teakowa/tgdl-bot/commit/c78a73a6c9e7f2580693331c61a2a120a1d5f20e))
* **storage:** accept float d1 migration duration metadata ([83f0002](https://github.com/Teakowa/tgdl-bot/commit/83f0002a5f94df7678b872ccfa81d4b61e489ed9))
* **task-lifecycle:** use tdl --ns and sync status messages ([98d44d8](https://github.com/Teakowa/tgdl-bot/commit/98d44d86844d152483cff6270f296c93071c9ccc))

## [1.1.0](https://github.com/Teakowa/tgdl-bot/compare/v1.0.0...v1.1.0) (2026-03-22)


### Features

* **bot:** add explicit forward target support ([39e4392](https://github.com/Teakowa/tgdl-bot/commit/39e43926c9a65b2d20e586cce106cc9b58ee5bed))
* **bot:** add queue action menus ([2754eb8](https://github.com/Teakowa/tgdl-bot/commit/2754eb886aae77f05892187567e986f006aa82ec))

## 1.0.0 (2026-03-22)


### Features

* add config and entrypoint skeletons ([cfb076b](https://github.com/Teakowa/tgdl-bot/commit/cfb076b33908d4db400699b1749f336e7c72e811))
* **bot:** add queue/delete/retry commands with inline callbacks ([21c1c50](https://github.com/Teakowa/tgdl-bot/commit/21c1c503fc5487c8a2555b1235cc0f7e0fece92e))
* **bot:** add webhook mode with polling fallback ([e6bda4f](https://github.com/Teakowa/tgdl-bot/commit/e6bda4f75fbb7f22e616a79e504fa3732495795d))
* **bot:** rebuild failed duplicate tasks and add reaction feedback ([cc9ed1d](https://github.com/Teakowa/tgdl-bot/commit/cc9ed1da15fcc7dc1a4935d8576225f79a98d930))
* **bot:** support force delete for non-running tasks ([cf63e65](https://github.com/Teakowa/tgdl-bot/commit/cf63e654bb9d083255720fb5822ffb8049460bc9))
* **deploy:** add single-image docker runtime with embedded tdl ([6a126d9](https://github.com/Teakowa/tgdl-bot/commit/6a126d97a67ee51ce3d188a9717479b39fde678e))
* implement phase 1 task pipeline and downloader consumer ([141eace](https://github.com/Teakowa/tgdl-bot/commit/141eaceea45d6baa3a2bb1dc52d48ccf39577749))
* **observability:** add bot/downloader lifecycle logs for compose ([3a61bbc](https://github.com/Teakowa/tgdl-bot/commit/3a61bbc22db88a0efc496d88364d7d0b4465f5d4))
* **phase2:** switch task model from download to forwarding ([31c58cd](https://github.com/Teakowa/tgdl-bot/commit/31c58cdb8692ae241cb34888b2dd8c7657ff8c04))
* retry failed tasks on startup and quote bot replies ([41ec61b](https://github.com/Teakowa/tgdl-bot/commit/41ec61b9d85dd7090bac4343198cc8efa02c539e))
* **scaffold:** initialize phase1 tgdl bot project skeleton ([6758814](https://github.com/Teakowa/tgdl-bot/commit/67588147da9f0c6bf43d02f587053b4289a5b357))
* **storage:** migrate task persistence to D1 and split deployment compose ([5bc2461](https://github.com/Teakowa/tgdl-bot/commit/5bc2461ebf5da287d1d0bd5eb2924b54ae95f802))
* **sync:** move task status updates to bot via status queue ([e85e44f](https://github.com/Teakowa/tgdl-bot/commit/e85e44f593df99455697e41aba0c5969f4227e99))


### Bug Fixes

* **bot:** add button selection to delete command ([0d7a50f](https://github.com/Teakowa/tgdl-bot/commit/0d7a50fbbc9da68b04dd8fd99e684ad4e259fc75))
* **bot:** skip redundant initial status sync ([8053fd1](https://github.com/Teakowa/tgdl-bot/commit/8053fd1c7a18756935aacd45a865f437ff0ffa33))
* **bot:** use Telegram-compatible reaction emojis ([30c6690](https://github.com/Teakowa/tgdl-bot/commit/30c6690c6b75cc7278336709232a69d151aa1b75))
* **downloader:** collapse byte-based tdl progress logs ([d306ebe](https://github.com/Teakowa/tgdl-bot/commit/d306ebe67085a2bd24c34904534747c128ac6ff0))
* **downloader:** enforce single active tdl executor ([c8081fc](https://github.com/Teakowa/tgdl-bot/commit/c8081fc3e008b9abedfeaa44db08f8e96a33f435))
* **downloader:** reduce tdl progress stream log noise ([a0eb83c](https://github.com/Teakowa/tgdl-bot/commit/a0eb83ce6376453fd039636198f37d81ef763ecb))
* **downloader:** refresh task before final status notify ([b5db396](https://github.com/Teakowa/tgdl-bot/commit/b5db3964648e103e90591a5c123caaf59d5109a9))
* **downloader:** retry transient tdl interruptions and enable reconnect ([f3b784b](https://github.com/Teakowa/tgdl-bot/commit/f3b784b3c44e068f6f86a585978c8baa00154ce5))
* **downloader:** stream tdl stdout stderr logs ([546bd95](https://github.com/Teakowa/tgdl-bot/commit/546bd957f5c1c2c02d229cf563748566cadd9bbb))
* **downloader:** switch tdl forward to --from and tighten cli error classification ([ce9f002](https://github.com/Teakowa/tgdl-bot/commit/ce9f00230a8be7e152a016b60d53aa311b9e085f))
* **forward:** make target chat optional for saved messages default ([18b7e25](https://github.com/Teakowa/tgdl-bot/commit/18b7e25d87cfa12f7ac187b3bfab673e3d6e3e51))
* implement real tdl preflight and stabilize sqlite startup ([761fdd6](https://github.com/Teakowa/tgdl-bot/commit/761fdd6236b195d90eb9ecf03c95abecb0ec5039))
* parse Telegram message date as unix timestamp ([898f552](https://github.com/Teakowa/tgdl-bot/commit/898f552eca814eafe75a4c4c2314c4977d91031b))
* **queue:** align pull and ack/retry with Cloudflare contracts ([16c169d](https://github.com/Teakowa/tgdl-bot/commit/16c169d6cf7a82c60bd15822812d5faa97a75b67))
* **queue:** use single-message push contract for enqueue ([53a82e4](https://github.com/Teakowa/tgdl-bot/commit/53a82e4ed109f3e2153e0005081bfaffbf7fabd6))
* **queue:** wrap push messages with body for Cloudflare API ([5a25ef5](https://github.com/Teakowa/tgdl-bot/commit/5a25ef5a06b66a19be9e274ba48b255ff7ff2f2d))
* **reaction:** preserve final status when refreshing message ids ([794c3c9](https://github.com/Teakowa/tgdl-bot/commit/794c3c995e716ae2c62a7241d9d4eef40130d963))
* **reaction:** use telegram-compatible emojis for source message ([c78a73a](https://github.com/Teakowa/tgdl-bot/commit/c78a73a6c9e7f2580693331c61a2a120a1d5f20e))
* **storage:** accept float d1 migration duration metadata ([83f0002](https://github.com/Teakowa/tgdl-bot/commit/83f0002a5f94df7678b872ccfa81d4b61e489ed9))
* **task-lifecycle:** use tdl --ns and sync status messages ([98d44d8](https://github.com/Teakowa/tgdl-bot/commit/98d44d86844d152483cff6270f296c93071c9ccc))
