//go:build windows

package windowsclient

import (
 "fmt"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
)

// N4NativeWFPReader is strictly read-only. WFP filter installation,
// persistence and ownership recovery are separate native safety gates.
type N4NativeWFPReader struct{}

var n4WFP=windows.NewLazySystemDLL("fwpuclnt.dll")
var (
 n4WFPOpen=n4WFP.NewProc("FwpmEngineOpen0")
 n4WFPClose=n4WFP.NewProc("FwpmEngineClose0")
 n4WFPLayerGet=n4WFP.NewProc("FwpmLayerGetByKey0")
 n4WFPFree=n4WFP.NewProc("FwpmFreeMemory0")
)
var (
 n4LayerConnectV6=windows.GUID{Data1:0x4a72393b,Data2:0x319f,Data3:0x44bc,Data4:[8]byte{0x84,0xc3,0xba,0x54,0xdc,0xb3,0xb6,0xb4}}
 n4LayerReceiveV6=windows.GUID{Data1:0xa3b42c97,Data2:0x9f04,Data3:0x4672,Data4:[8]byte{0xb8,0x7e,0xce,0xe9,0xc4,0x83,0x25,0x7f}}
 n4LayerConnectV4=windows.GUID{Data1:0xc38d57d1,Data2:0x05a7,Data3:0x4c33,Data4:[8]byte{0x90,0x4f,0x7f,0xbc,0xee,0xe6,0x0e,0x82}}
)
func (N4NativeWFPReader) VerifyLayers()error{
 for _,p:=range []*windows.LazyProc{n4WFPOpen,n4WFPClose,n4WFPLayerGet,n4WFPFree}{
  if err:=p.Find();err!=nil{return fmt.Errorf("%w: %v",ErrN4UnsupportedWindows,err)}
 }
 var engine uintptr
 // RPC_C_AUTHN_WINNT=10; NULL optional auth identity and session. Opening
 // this WFP engine handle makes no filter/policy changes.
 rc,_,_:=n4WFPOpen.Call(0,10,0,0,uintptr(unsafe.Pointer(&engine)))
 if rc!=0{return fmt.Errorf("FwpmEngineOpen0 read-only: %w",syscall.Errno(rc))}
 if engine==0{return ErrN4NativeGuard}
 defer n4WFPClose.Call(engine)
 for _,guid:=range []windows.GUID{n4LayerConnectV6,n4LayerReceiveV6,n4LayerConnectV4}{
  var layer uintptr
  rc,_,_:=n4WFPLayerGet.Call(engine,uintptr(unsafe.Pointer(&guid)),uintptr(unsafe.Pointer(&layer)))
  if rc!=0 {return fmt.Errorf("FwpmLayerGetByKey0 %v: %w",guid,syscall.Errno(rc))}
  if layer==0{return ErrN4NativeGuard}
  n4WFPFree.Call(uintptr(unsafe.Pointer(&layer)))
 }
 return nil
}
