//go:build linux

package linuxserver

import (
	"net"
	"os"
)

func NotifyReady() error {
	path:=os.Getenv("NOTIFY_SOCKET");if path==""{return nil}
	c,err:=net.DialUnix("unixgram",nil,&net.UnixAddr{Name:path,Net:"unixgram"});if err!=nil{return err};defer c.Close()
	_,err=c.Write([]byte("READY=1\nSTATUS=Shared TUN and raw endpoint ready"));return err
}
