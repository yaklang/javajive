# Adversarial DEX corpus manifest

`manifest.json` pins Arsylk/dexfile test artifacts to commit `98e55b86ebb26dd8e014afc6fb61fdeb697dc663`. Corpus binaries are intentionally not stored here. DEX 040 is explicit but pending because this upstream commit has no `v040.dex`; CompactDex is a separate pending format family because the commit has no CompactDex artifact.

Fetch into a user-selected cache, verifying every available artifact's exact size and SHA-256:

```sh
go run ./tools/dex_corpus fetch -manifest dex2jar/testdata/adversarial/manifest.json -cache /path/to/cache
```

Probe the verified cache with bounded native `DexToJar` workers and write a deterministic JSON matrix:

```sh
go run ./tools/dex_corpus probe -manifest dex2jar/testdata/adversarial/manifest.json -cache /path/to/cache > matrix.json
```

The probe never executes input or translated code. Each artifact runs in a subprocess with a 10-second deadline, 16 MiB input/expanded-DEX bounds, and a 32-entry multidex bound. Output contains dispositions, failure stages, categories, diagnostics, and successful conversion counters; it omits timings and generated JAR bytes.

All pinned artifacts and the included license text are MIT licensed by Arsylk. The license source is the same commit's immutable `LICENSE` blob.
