package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	_ "golang.org/x/image/webp"
	"image"
	"image/jpeg"
	_ "image/png"
	"io/ioutil"
	"math"
	"net/http"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/corona10/goimagehash"
	"github.com/dghubble/go-twitter/twitter"
	"github.com/dghubble/oauth1"
	"github.com/vartanbeno/go-reddit/v2/reddit"
)

type Conf struct {
	Api struct {
		Reddit struct {
			Subs []string `json:"subreddits"`
			User string   `json:"account_username"`
			Pass string   `json:"account_password"`
			ID   string   `json:"app_id"`
			Sec  string   `json:"app_secret"`
		}
		Twitter struct {
			Token string `json:"access_token"`
			ToknS string `json:"access_token_secret"`
			Conk  string `json:"api_key"`
			Cons  string `json:"api_secret_key"`
		} `json:"twitter"`
	} `json:api`
	Scan struct {
		Dept int     `json:"strictness"`
		Mult float64 `json:"speed"`
	} `json:"analysis"`
}

var ctx = context.Background()

func loadData(configFile string) (map[string]struct{}, map[string]struct{}, map[string][]string, Conf, *os.File) {
	fmt.Println("Loading " + configFile + "...")

	var config Conf
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to read config file! Error:\n", err)
		os.Exit(1)
	}
	if json.Unmarshal(data, &config) != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to parse config file! Error:\n", err)
		os.Exit(1)
	}
	fmt.Println("Program configuration loaded. Loading post database...")

	f, err := os.OpenFile("posts.csv", os.O_APPEND|os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to open database file! Error:\n", err)
		os.Exit(1)
	}

	idset, hashset, chashset := make(map[string]struct{}), make(map[string]struct{}), make(map[string][]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		postinfo := strings.Split(scanner.Text(), ",")

		longhashes := []string{}
		shorthashes := []string{}
		for index, data := range postinfo {
			switch {
			case strings.HasPrefix(data, "t3_"):
				idset[data] = struct{}{}
			case strings.HasPrefix(data, "p:") && len(data) == 18:
				shorthashes = append(shorthashes, data)
			case strings.HasPrefix(data, "p:") && len(data) == 66:
				longhashes = append(longhashes, data)
				hashset[data] = struct{}{}
			case index == 0 && len(data) == 6: // Needed for backwards compatibility with Tootgo and databases from previous versions
				idparts := []string{"t3", postinfo[0]}
				idset[strings.Join(idparts, "_")] = struct{}{}
			default:
				//fmt.Println("Warn: Ignoring invalid data \"" + data + "\" in database file.")
			}
		}
		for _, shorthash := range shorthashes {
			chashset[shorthash] = append(chashset[shorthash], longhashes...)
		}
	}
	fmt.Println(len(idset), "post IDs loaded into memory,", len(hashset)+len(chashset), "image hashes loaded into memory.\n\n")
	return idset, hashset, chashset, config, f
}

func getRedditPosts(config Conf) []*reddit.Post {
	var subreddit string
	for i, sub := range config.Api.Reddit.Subs {
		if i == 0 {
			subreddit = sub
		} else if sub != "" {
			subreddit = subreddit + "+" + sub
		}
	}
	if len(subreddit) == 0 {
		fmt.Fprintln(os.Stderr, "Fatal: config.reddit.subreddits must have a length greater than zero!")
		os.Exit(1)
	}

	rclient, err := reddit.NewClient(reddit.Credentials{
		ID:       config.Api.Reddit.ID,
		Secret:   config.Api.Reddit.Sec,
		Username: config.Api.Reddit.User,
		Password: config.Api.Reddit.Pass,
	}, reddit.WithUserAgent("redditEsp"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to create Reddit client! Error:\n", err)
		os.Exit(1)
	}

	fmt.Println("Downloading list of \"hot\" posts on /r/" + subreddit + "...")
	posts, resp, err := rclient.Subreddit.HotPosts(ctx, subreddit, &reddit.ListOptions{
		Limit: 100,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to download post list! Error:\n", err)
		os.Exit(1)
	}

	resp.Body.Close()
	return posts
}

func isImageURL(url string) bool {
	return strings.HasSuffix(url, ".png") || strings.HasSuffix(url, ".jpg") || strings.HasPrefix(url, "https://imgur.com/") || strings.HasPrefix(url, "http://imgur.com/")
}

func filterRedditPosts(config Conf, posts []*reddit.Post) []*reddit.Post {
	fmt.Println("Downloaded post list. Analyzing and filtering posts...")

	var upvoteRatios, upvoteRates, scores, ages []int
	for _, post := range posts {
		if post.IsSelfPost || post.Stickied || post.Locked || !isImageURL(post.URL) || len(post.Title) > 257 {
			// None of these are very useful to an image mirroring bot.
			continue
		}
		if len(scores) >= config.Scan.Dept {
			break
		}

		upvoteRatios = append(upvoteRatios, int(post.UpvoteRatio*100))
		upvoteRates = append(upvoteRates, int(float64(post.Score)/time.Since(post.Created.Time).Hours()))
		scores = append(scores, post.Score)
		ages = append(ages, int(time.Now().UTC().Sub(post.Created.Time.UTC()).Seconds()))
	}

	sort.Ints(upvoteRatios)
	sort.Ints(upvoteRates)
	sort.Ints(scores)
	sort.Ints(ages)

	upvoteRatioTarget, upvoteRateTarget, scoreTarget, ageTargetMin, ageTargetMax, rankTarget := 0, 0, 0, time.Duration(0), time.Duration(168)*time.Hour, len(scores)

	if len(scores) > 6 {
		scoreTarget = scores[(len(scores)/3)-1]
		upvoteRateTarget = upvoteRates[(len(upvoteRates)/3)-1]
		fmt.Println(len(scores), "posts were usable for image mirroring.\nCurrent posting criteria:\n\tMinimum upvotes:", scoreTarget, "\n\tMinimum upvote rate:", upvoteRateTarget, "upvotes/hour")
	}
	if len(upvoteRatios) > 10 {
		upvoteRatioTarget = upvoteRatios[(len(upvoteRatios)/10)-1]
		fmt.Println("\tMinimum upvote to downvote ratio:", float32(upvoteRatioTarget)/100)
	}
	if len(ages) > 6 {
		ageTargetMin = time.Duration(ages[(len(ages)/6)-1]) * time.Second
		fmt.Println("\tMinimum post age:", ageTargetMin.Round(time.Second))
	}
	if len(ages) > 30 {
		ageTargetMax = time.Duration(ages[(len(ages)-1)-(len(ages)/30)]) * time.Second
		fmt.Println("\tMaximum post age:", ageTargetMax.Round(time.Second))
	}
	if len(scores) > 60 {
		rankTarget = int(float64(len(scores)) - (math.Pow((float64(len(scores))/15), 2) - float64(len(scores))/4))
		fmt.Println("\tMaximum post rank:", rankTarget)
	}

	var goodPosts []*reddit.Post
	for i, post := range posts {
		if post.IsSelfPost || post.Stickied || post.Locked || !isImageURL(post.URL) || len(post.Title) > 257 || int(post.UpvoteRatio*100) < upvoteRatioTarget || post.Score < scoreTarget || float64(post.Score)/time.Since(post.Created.Time).Hours() < float64(upvoteRateTarget) || time.Now().UTC().Sub(post.Created.Time.UTC()) < ageTargetMin || time.Now().UTC().Sub(post.Created.Time.UTC()) > ageTargetMax {
			continue
		}
		goodPosts = append(goodPosts, post)
		if i > rankTarget {
			break
		}
	}
	if len(scores) > 6 {
		fmt.Println(len(goodPosts), "/", len(scores), "posts met the automatically selected posting critera.")
	} else if len(scores) == 0 {
		fmt.Fprintln(os.Stderr, "Warn: No posts were usable for image mirroring.")
		os.Exit(0)
	} else {
		fmt.Println(len(goodPosts), "posts were useable for image mirroring.")
	}

	return goodPosts
}

func downloadImageURL(url string) (*http.Response, string, error) {
	if strings.HasPrefix(url, "http://imgur.com/") {
		url = "https://i.imgur.com/" + strings.TrimPrefix(url, "http://imgur.com/") + ".jpg"
	}
	if strings.HasPrefix(url, "https://imgur.com/") {
		url = "https://i.imgur.com/" + strings.TrimPrefix(url, "https://imgur.com/") + ".jpg"
	}

	fmt.Println("Downloading image from", url+"...")
	resp, err := http.DefaultClient.Get(url)

	return resp, url, err
}

func getUniqueRedditPost(posts []*reddit.Post, f *os.File, idset map[string]struct{}, hashset map[string]struct{}, chashset map[string][]string, postLimit int) (*reddit.Post, []byte, string) {
	if len(posts) > postLimit {
		fmt.Println("Limiting search depth to", postLimit, "posts.")
	} else {
		postLimit = len(posts)
	}

	for i, post := range posts {
		if i > postLimit {
			break
		}

		_, ok := idset[post.FullID]
		if ok {
			continue
		}

		fmt.Println("\nPotentially unique post", post.ID, "found at a post depth of", i, "/", postLimit)

		resp, url, err := downloadImageURL(post.URL)
		if err != nil || resp.StatusCode >= 500 {
			fmt.Fprintln(os.Stderr, "Warn: Unable to download image! Error:\n", err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			fmt.Println("Unable to download " + path.Base(url) + "! Skipping post and adding ID to database.")

			idset[post.FullID] = struct{}{}
			f.WriteString(post.FullID + "\n")

			fmt.Println("Database now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
			continue
		}

		fmt.Println("Decoding and hashing", path.Base(url)+"...")

		imageData, imageType, err := image.Decode(resp.Body)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warn: Unable to decode image! Error:\n", err)

			idset[post.FullID] = struct{}{}
			f.WriteString(post.FullID + "\n")

			fmt.Println("Skipping post and adding ID to database.\nDatabase now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
			continue
		}

		hashraw, err := goimagehash.ExtPerceptionHash(imageData, 16, 16)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warn: Unable to hash image! Error:\n", err)

			idset[post.FullID] = struct{}{}
			f.WriteString(post.FullID + "\n")

			fmt.Println("Skipping post and adding ID to database.\nDatabase now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
			continue
		}
		hash := hashraw.ToString()

		chashraw, err := goimagehash.ExtPerceptionHash(imageData, 8, 8)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Warn: Unable to hash image! Error:\n", err)

			idset[post.FullID] = struct{}{}
			f.WriteString(post.FullID + "\n")

			fmt.Println("Skipping post and adding ID to database.\nDatabase now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
			continue
		}
		chash := chashraw.ToString()

		_, ok = hashset[hash]
		if ok {
			fmt.Println("Duplicate image detected (method: strict perceptual), skipping post and adding ID to database.")

			idset[post.FullID] = struct{}{}
			f.WriteString(post.FullID + "\n")

			fmt.Println("Database now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
			continue
		}

		val, ok := chashset[chash]
		if ok {
			distance := 255
			for _, phash := range val {
				rawphash, err := goimagehash.ExtImageHashFromString(phash)
				if err != nil {
					continue
				}

				dist, err := hashraw.Distance(rawphash)
				if err != nil {
					continue
				}

				if distance > dist {
					distance = dist
				}
			}
			if distance <= 25 {
				fmt.Println("Similar image detected (method: fuzzy perceptual, hash similarity: " + strconv.FormatFloat((float64(254-distance)/255)*100, 'f', 2, 64) + "%), skipping post and adding ID+hash to database.")

				idset[post.FullID] = struct{}{}
				hashset[hash] = struct{}{}
				chashset[chash] = append(chashset[chash], hash)
				f.WriteString(post.FullID + "," + hash + "," + chash + "\n")

				fmt.Println("Database now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")
				continue
			}
		}

		var buf bytes.Buffer
		jpeg.Encode(&buf, imageData, &jpeg.Options{
			/* Theoretically, yes, there is some *slight* generation loss from lossy re-encoding.
			However, with JPEG quality 100, the generation loss is imperceptable to humans, even after many re-encodes.*/
			Quality: 100,
		})

		fmt.Println("Image (type: " + imageType + ") is unique, adding ID and hash to database...")

		idset[post.FullID] = struct{}{}
		hashset[hash] = struct{}{}
		chashset[chash] = append(chashset[chash], hash)
		f.WriteString(post.FullID + "," + hash + "," + chash + "\n")

		fmt.Println("Database now contains", len(idset), "post IDs and", len(hashset)+len(chashset), "hashes.\n")

		return post, buf.Bytes(), path.Base(url)
	}
	fmt.Fprintln(os.Stderr, "\nWarn: No unique posts were found.")
	
    dt := time.Now()
    fmt.Println("\n["+dt.Format(time.Kitchen)+"] Waiting 30 minutes to repost...")

    time.Sleep(1800 * time.Second)
    main()

	return nil, nil, "" // Still have to return something, even if os.Exit() is being called.
}

func createTwitterPost(config Conf, post *reddit.Post, eimg []byte, file string) {
	fmt.Println("Uploading", file, "to twitter...")

	oconfig := oauth1.NewConfig(config.Api.Twitter.Conk, config.Api.Twitter.Cons)
	token := oauth1.NewToken(config.Api.Twitter.Token, config.Api.Twitter.ToknS)
	tclient := twitter.NewClient(oconfig.Client(oauth1.NoContext, token))

	res, resp, err := tclient.Media.Upload(eimg, "image/jpeg")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to upload image to Twitter! Error:\n", err)
		os.Exit(1)
	}
	resp.Body.Close()
    
    jastags := "#reddit #redditesp #ibai #rubius #lmdshow #folagor #cristinini #elmillor #orslok #yointerneto"

	fmt.Println("Creating tweet (PostID: " + post.ID + ")...")
	isNSFW := post.NSFW || post.Spoiler
    tweet, resp, err := tclient.Statuses.Update(post.Title+" https://redd.it/"+post.ID+" ("+post.SubredditNamePrefixed+")\n\n"+jastags, &twitter.StatusUpdateParams{
		MediaIds:          []int64{res.MediaID},
		PossiblySensitive: &isNSFW,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "Fatal: Unable to create Tweet! Error:\n", err)
		os.Exit(1)
	}
	resp.Body.Close()

	fmt.Println("Tweet:\n\t"+tweet.Text, "\n\thttps://twitter.com/"+tweet.User.ScreenName+"/status/"+tweet.IDStr)
}

func main() {
	configFile := "conf.json"
	depthLimit := 50
	if len(os.Args[1:]) > 0 {
		configFile = os.Args[1]
	}

	idset, hashset, chashset, config, rawDB := loadData(configFile)
	defer rawDB.Close()

	finfo, err := rawDB.Stat()
	if err == nil {
		sinceMod := time.Since(finfo.ModTime()).Minutes()
		depthLimit = int(config.Scan.Mult * math.Sqrt(sinceMod))
		if depthLimit < int(config.Scan.Mult) {
			depthLimit = int(config.Scan.Mult)
		}
	}

	http.DefaultClient.Timeout = 30 * time.Second
	posts := filterRedditPosts(config, getRedditPosts(config))
	post, image, imageName := getUniqueRedditPost(posts, rawDB, idset, hashset, chashset, depthLimit)
	createTwitterPost(config, post, image, imageName)
    
    dt := time.Now()    
    fmt.Println("\n["+dt.Format(time.Kitchen)+"] Waiting 30 minutes to repost...")
    
    time.Sleep(1800 * time.Second)    
    main()
}

/*** TIME INFORMATION
const (
    ANSIC       = "Mon Jan _2 15:04:05 2006"
    UnixDate    = "Mon Jan _2 15:04:05 MST 2006"
    RubyDate    = "Mon Jan 02 15:04:05 -0700 2006"
    RFC822      = "02 Jan 06 15:04 MST"
    RFC822Z     = "02 Jan 06 15:04 -0700" // RFC822 with numeric zone
    RFC850      = "Monday, 02-Jan-06 15:04:05 MST"
    RFC1123     = "Mon, 02 Jan 2006 15:04:05 MST"
    RFC1123Z    = "Mon, 02 Jan 2006 15:04:05 -0700" // RFC1123 with numeric zone
    RFC3339     = "2006-01-02T15:04:05Z07:00"
    RFC3339Nano = "2006-01-02T15:04:05.999999999Z07:00"
    Kitchen     = "3:04PM"
    // Handy time stamps.
    Stamp      = "Jan _2 15:04:05"
    StampMilli = "Jan _2 15:04:05.000"
    StampMicro = "Jan _2 15:04:05.000000"
    StampNano  = "Jan _2 15:04:05.000000000"
)
***/
