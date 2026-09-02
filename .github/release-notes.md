The dashboard, as a command and as an application.

| download | what it is |
| --- | --- |
| `Grove-macos-universal.zip` | the app, Apple Silicon and Intel in one bundle |
| `Grove-windows-amd64.zip` | the app on Windows |
| `grove-cli-*` | the command, which serves the same dashboard to a browser |

**macOS, first run.** These builds are not notarised by Apple, so macOS
quarantines them however they arrive — downloaded or installed by Homebrew
alike — and refuses to open them. Opening one anyway gives a dialog saying
macOS "could not verify Grove.app is free of malware", whose buttons are
**Done** and **Move to Bin**. There is no "Open Anyway" in it.

So, with Homebrew:

```sh
brew tap programmfabrik/grove https://github.com/programmfabrik/grove
brew trust --cask programmfabrik/grove/grove
brew install --cask grove
xattr -dr com.apple.quarantine /Applications/Grove.app
```

…or, having unzipped `Grove.app` into `/Applications` yourself, the last line
on its own. None of the three extra lines is optional: Homebrew refuses a cask
from a third-party tap until you trust it, and it quarantines its downloads
exactly as a browser does.

If you have already met that dialog and moved Grove to the Bin, nothing is
wrong with it — put it back, run the `xattr` line, and open it again.

**Windows, first run.** SmartScreen warns about a program it has not seen
before: **More info → Run anyway**.

Both are the cost of an unsigned build and neither says anything about the
program. Signing is on the list.

grove needs `git` on the path and reads nothing else.
