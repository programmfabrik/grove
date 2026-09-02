The dashboard, as a command and as an application.

| download | what it is |
| --- | --- |
| `Grove-macos-universal.zip` | the app, Apple Silicon and Intel in one bundle |
| `Grove-windows-amd64.zip` | the app on Windows |
| `grove-cli-*` | the command, which serves the same dashboard to a browser |

**macOS, first run.** These builds are not signed with a Developer ID, so
macOS quarantines them on download and refuses to open them. Either install
through Homebrew, which does not go through the quarantine:

```sh
brew tap programmfabrik/grove https://github.com/programmfabrik/grove
brew install --cask grove
```

…or, having unzipped `Grove.app` into `/Applications`, tell macOS you meant it:

```sh
xattr -dr com.apple.quarantine /Applications/Grove.app
```

Opening it from Finder without that shows a warning; **System Settings →
Privacy & Security → Open Anyway** is the way through it.

**Windows, first run.** SmartScreen warns about a program it has not seen
before: **More info → Run anyway**.

Both are the cost of an unsigned build and neither says anything about the
program. Signing is on the list.

grove needs `git` on the path and reads nothing else.
