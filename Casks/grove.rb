# The template the release workflow fills in: it knows the version and the
# checksum of the file it just built, and this file cannot know either.
cask "grove" do
  version "0.1.0"
  sha256 "cf8723f04996d70fe346cd16c8e92713f48410a649f6981ac7ec35df1af1a07c"

  url "https://github.com/programmfabrik/grove/releases/download/v#{version}/Grove-macos-universal.zip"
  name "Grove"
  desc "One page over a directory full of git repositories"
  homepage "https://github.com/programmfabrik/grove"

  depends_on macos: ">= :big_sur"

  app "Grove.app"
  binary "#{appdir}/Grove.app/Contents/MacOS/Grove", target: "grove"

  zap trash: [
    "~/Library/Application Support/grove",
  ]
end
