module github.com/Arash-Afshar/backlite/cmd

go 1.24.4

replace github.com/mikestefanello/backlite => ../

require (
	github.com/mattn/go-sqlite3 v1.14.28
	github.com/mikestefanello/backlite v0.5.0
)

require github.com/google/uuid v1.6.0 // indirect
