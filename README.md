## KatMirrorBot
KatMirrorBot is an image mirroring bot that tries to mirror the *best* content from Reddit to Twitter. Used by [@it_meirl_bot](https://twitter.com/it_meirl_bot).

## Features
- Extremely simple configuration.
- Only runs during posting, freeing up idle memory usage for other programs.
- Easily recovers from crashes and network errors, without leaving behind temporary files.
- Human-readable and easily editable data storage format, fully backwards-compatible with *all* KatMirrorBot versions, and fully forwards+backwards compatible with all KatMirrorBot versions since commit 96e6b66.
  - Note: Backwards compatibility is only for the data format. The configuration is subject to change over time, however, it should be near-trivial to migrate config files between versions (starting with commit acf67ee).
- Automatically detects the best "hot" posts to mirror, based on various metrics like upvotes, upvote rate, upvote:downvote ratio, post age, and post depth.
- Automatic post criteria detection based on other subreddit posts.
- Automatically adjusts search depth based on posting frequency.
- Detects and prevents "reposts" (duplicate images) from being uploaded to the bot account.
- Uses advanced duplicate detection (DCT-based image hashes) to find duplicates that are visually similar, but have slight alterations (scaling artifacts, color changes, lossy compression artifacts, slight cropping)
- Automatically fixes corrupted images, and discards those that can't be fixed.
- Automatically optimized uploaded images for Twitter.

## Compiling
1. Install and setup a [Golang compiler](https://golang.org/).
2. Download the latest code through Git (`git clone https://github.com/katattakd/KatMirrorBot.git`) or through [GitHub](https://github.com/katattakd/KatMirrorBot/archive/main.zip).
3. Run the command `go build -ldflags="-s -w" -tags netgo` to download dependencies and compile a small static binary. Ommit `-ldflags="-s -w"` if you intend to run a debugger on the program, and ommit `-tags netgo` if a static binary isn't necessary.

## Usage
1. Get API keys from [developer.twitter.com](https://developer.twitter.com/en) and [reddit.com/prefs/apps](https://www.reddit.com/prefs/apps). How to do this is outside the scope of this README.
2. Edit the conf.json file. Fill in your API keys and the subreddits you want to mirror (by default, the program mirrors [/r/all](https://www.reddit.com/r/all) and [/r/popular](https://www.reddit.com/r/popular).
3. Test your configuration by running the KatMirrorBot in your terminal.
4. Move your configuration file, database file, and the KatMirrorBot folder (but not it's contents) to your user's home folder.
5. Add the following lines to your crontab (`crontab -e`):
```crontab
*/3 * * * * ~/KatMirrorBot/KatMirrorBot >> mirror.log 2>&1
0 0 * * * /bin/rm mirror.log
```
With the default `config.analysis.strictness` and `config.analysis.speed`, this should function almost identically to @it_meirl_bot's current configuration.

### Advanced usage
- `config.analysis.strictness` changes how many posts are analyzed. A lower number makes the bot more strict with what posts it accepts, however, a very low analysis depth reduces the amount of analysis the program can perform. Should be between 50-100 posts.
- `config.analysis.speed` changes how the post search depth is calculated. A higher number will make the bot increase the search depth much faster, and a lower number will make the bot increase the search depth much slower. Has no units, should be between 4-15.
- If you intend to edit the stored posts, they're stored in the `posts.csv` file. Posts are seperated by newlines, and individual data points for a post are seperated by commas. A post can have any of the following data points, specified in any order (all are optional):
  - Data points starting with `t3_` are post IDs.
    - If the data point does not start with `t3_`, but is exactly 6 characters long and is the first data point in the row, it's also interpreted as a post ID. This is done for backwards compatibility with Tootgo.
  - Data points starting with `p:` are perceptual hashes. KatMirrorBot uses both 64-bit and 256-bit hashes.
- The name of the program's config file can be specified as an argument. If omitted, the config file defaults to `conf.json`.
