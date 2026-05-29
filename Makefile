.PHONY: sshpiper
sshpiper:
	cd sshpiper/sshpiper && go build -o ../../out/ ./cmd/sshpiperd

plugins:
	cd sshpiper && go build -o ../out/ ./plugins/...
