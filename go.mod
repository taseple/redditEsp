module github.com/iwuaizl/redditEsp

replace github.com/dghubble/go-twitter => ./go-twitter

GOVERSION=go1.17.6

go 1.17.6

require (
	github.com/cenkalti/backoff v2.2.1+incompatible // indirect
	github.com/corona10/goimagehash v1.0.3
	github.com/dghubble/go-twitter v0.0.0-20210609183100-2fdbf421508e
	github.com/dghubble/oauth1 v0.7.0
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/vartanbeno/go-reddit/v2 v2.0.1
	golang.org/x/image v0.0.0-20210622092929-e6eecd499c2c
	golang.org/x/net v0.0.0-20210614182718-04defd469f4e // indirect
	golang.org/x/oauth2 v0.0.0-20210622215436-a8dc77f794b6 // indirect
	google.golang.org/appengine v1.6.7 // indirect
)
