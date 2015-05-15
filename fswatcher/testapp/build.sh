
# go get github.com/mitchellh/gox
gox -osarch="windows/amd64 linux/amd64" -output="fswatch_test_{{.OS}}"
