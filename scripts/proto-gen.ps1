$gopath = go env GOPATH
$plugin_path = "$gopath\bin\protoc-gen-go.exe"
& .\bin\protoc\bin\protoc.exe --plugin=protoc-gen-go=$plugin_path --go_out=. --go_opt=paths=source_relative pkg/protocol/messages.proto
