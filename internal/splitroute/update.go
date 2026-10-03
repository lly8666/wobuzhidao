package splitroute

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const UpdateURL = "https://raw.githubusercontent.com/gaoyifan/china-operator-ip/ip-lists/china.txt"

// Update is explicit, bounded, and does not touch the active kernel snapshot.
// Download/validation failure leaves an existing file intact. Restart to apply.
func Update(path string) error {
	c := &http.Client{Timeout: 30*time.Second, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if len(via)>=3 || r.URL.Scheme!="https" || r.URL.Host!="raw.githubusercontent.com" { return fmt.Errorf("splitroute: refused update redirect") }; return nil
	}}
	r,err:=c.Get(UpdateURL); if err!=nil { return fmt.Errorf("splitroute: update download failed") }; defer r.Body.Close()
	if r.StatusCode!=http.StatusOK { return fmt.Errorf("splitroute: update HTTP %d",r.StatusCode) }
	raw,err:=io.ReadAll(io.LimitReader(r.Body,MaxBytes+1)); if err!=nil || len(raw)>MaxBytes { return fmt.Errorf("splitroute: update read failed or too large") }
	return WriteSnapshot(path,raw)
}

func WriteSnapshot(path string, raw []byte) error {
	pp,err:=Parse(bytes.NewReader(raw)); if err!=nil { return err }
	var b bytes.Buffer
	b.WriteString("# WBD China IPv4 manual snapshot; source="+UpdateURL+"\n")
	for _,p:=range pp { fmt.Fprintln(&b,p) }
	f,err:=os.CreateTemp(filepath.Dir(path),".wbd-cn-*.tmp"); if err!=nil { return err }
	name:=f.Name(); defer os.Remove(name)
	if _,err=f.Write(b.Bytes()); err!=nil { f.Close(); return err }
	if err=f.Sync(); err!=nil { f.Close(); return err }; if err=f.Close(); err!=nil { return err }
	if err=os.Rename(name,path); err!=nil { return err }
	fmt.Printf("WBD_CHINA_IP_UPDATED prefixes=%d sha256=%x restart_required=1\n",len(pp),sha256.Sum256(b.Bytes()))
	return nil
}
