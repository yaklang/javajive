# External rejection corpus

These files are pinned, byte-for-byte upstream test artifacts. They exercise explicit rejection paths; their presence here does not imply that javajive should translate them. Tests verify each SHA-256 before passing the file to the converter and do not access the network.

## Manifest

| Local file | Upstream source URL | Commit | SHA-256 | Size | License | Expected diagnostic |
| --- | --- | --- | --- | ---: | --- | --- |
| `arsylk-dexfile-tiny.dex` | <https://raw.githubusercontent.com/Arsylk/dexfile/98e55b86ebb26dd8e014afc6fb61fdeb697dc663/testdata/dex/tiny.dex> | Arsylk/dexfile `98e55b86ebb26dd8e014afc6fb61fdeb697dc663` | `613fc5ffac5c463f250d86be2355acdd2c261b052d2cdb12ce63efb479469145` | 1,072 bytes | MIT; see `licenses/Arsylk-dexfile-MIT.txt` | annotations: `classes.dex`, class `Lfixtures/tiny/Hello;`, offset `0x13c`, op `class_defs`, reason `unsupported annotations_off=0x354; must be zero` |
| `arsylk-dexfile-multidex.apk` | <https://raw.githubusercontent.com/Arsylk/dexfile/98e55b86ebb26dd8e014afc6fb61fdeb697dc663/testdata/dex/multidex.apk> | Arsylk/dexfile `98e55b86ebb26dd8e014afc6fb61fdeb697dc663` | `eb3c68fa2b12819e8388000422963ed13d08dce5b2dde4b78472239356036577` | 278,319 bytes | MIT; see `licenses/Arsylk-dexfile-MIT.txt` | annotations: APK entry `classes.dex`, class `Lfixtures/Arith;`, offset `0x41b4`, op `class_defs`, reason `unsupported annotations_off=0x42694; must be zero` |
| `pxb1988-dex2jar-dex038.dex` | <https://raw.githubusercontent.com/pxb1988/dex2jar/b5bda4fb4935ae8b3869b422454ae3b3896c7bc1/dex-translator/src/test/resources/dexes/dex038.dex> | pxb1988/dex2jar `b5bda4fb4935ae8b3869b422454ae3b3896c7bc1` | `368dd6e8ea8aaee26ad46438ccbef42d28295a4f4957379e34843472b2f13ea4` | 1,376 bytes | Apache-2.0; see `licenses/pxb1988-dex2jar-Apache-2.0.txt` and `licenses/pxb1988-dex2jar-NOTICE.txt` | version: `classes.dex`, offset `0x4`, op `header`, reason `unsupported DEX version "038"; only "035" is supported` |

## License provenance

The included texts were downloaded from these commit-pinned raw URLs:

- <https://raw.githubusercontent.com/Arsylk/dexfile/98e55b86ebb26dd8e014afc6fb61fdeb697dc663/LICENSE>
- <https://raw.githubusercontent.com/pxb1988/dex2jar/b5bda4fb4935ae8b3869b422454ae3b3896c7bc1/LICENSE.txt>
- <https://raw.githubusercontent.com/pxb1988/dex2jar/b5bda4fb4935ae8b3869b422454ae3b3896c7bc1/NOTICE.txt>
