.PHONY: proto
proto: proto/*.proto 
	# Generate Go code from the .proto files
	protoc --go_out=.  proto/messages.proto 
	mv project/polycast/* .
	rm -rf project