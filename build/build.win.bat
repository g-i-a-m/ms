go env -w GO111MODULE=on
go env -w GOPROXY=https://goproxy.io,direct
cd K:\pion-webrtc\examples\media-server
K:
del mediaserver.exe /q
go build -o mediaserver.exe mediaserver.go