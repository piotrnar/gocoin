module github.com/piotrnar/gocoin

go 1.16

require (
	github.com/seiflotfy/cuckoofilter v0.0.0-20220411075957-e3b120b3f5fb // indirect
	github.com/syndtr/goleveldb v1.0.0
)

replace github.com/syndtr/goleveldb => ./lib/others/goleveldb
