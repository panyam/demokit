# ansi_up — vendored

JS converter for ANSI SGR escape sequences into HTML. The
`<demokit-demo>` player imports `./ansi_up.js` to render colored
step output. Vendored so the player can resolve a relative-path
sibling import in every deployment shape (file://, served, embed).

| Field | Value |
|---|---|
| Upstream | https://github.com/drudru/ansi_up |
| Tag | v6.0.6 |
| Commit | `07a4824757d4dfbb41236a4245a6ce37f21aeb91` |
| License | MIT (see `ansi_up.LICENSE`) |
| sha256 (`ansi_up.js`) | `554a7d9ca4f3721db1f14941f92dc75b254f57d4b7bffeb84eea1174aa160780` |
| sha256 (`ansi_up.LICENSE`) | `605a7882fd6b556965dd1026fc55e3a5afa52b73b35d4e12fe9b4cf25b32d21f` |

## Verifying the file matches upstream

```bash
PIN=07a4824757d4dfbb41236a4245a6ce37f21aeb91
diff <(curl -sL "https://raw.githubusercontent.com/drudru/ansi_up/${PIN}/ansi_up.js") web/player/ansi_up.js
diff <(curl -sL "https://raw.githubusercontent.com/drudru/ansi_up/${PIN}/LICENSE")   web/player/ansi_up.LICENSE
shasum -a 256 web/player/ansi_up.js web/player/ansi_up.LICENSE
```

## Bumping the version

1. Pick a release tag from upstream releases.
2. Resolve to the commit SHA: `gh api repos/drudru/ansi_up/git/refs/tags/<tag> -q .object.sha`
3. Re-download from the pinned commit:
   ```bash
   PIN=<new-commit-sha>
   curl -sL "https://raw.githubusercontent.com/drudru/ansi_up/${PIN}/ansi_up.js" -o web/player/ansi_up.js
   curl -sL "https://raw.githubusercontent.com/drudru/ansi_up/${PIN}/LICENSE"   -o web/player/ansi_up.LICENSE
   shasum -a 256 web/player/ansi_up.js web/player/ansi_up.LICENSE
   ```
4. Update this file's commit SHA and sha256 fields.
5. Run the bundle smoke test to confirm the player renders ANSI output cleanly.
