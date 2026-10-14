//go:build windows

package windowsclient

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "errors"
 "fmt"
 "syscall"
 "unsafe"

 "golang.org/x/sys/windows"
 "golang.org/x/sys/windows/registry"
)

var (
 n4SetupAPI=windows.NewLazySystemDLL("setupapi.dll")
 n4SetupClassDevs=n4SetupAPI.NewProc("SetupDiGetClassDevsW")
 n4SetupEnumDevice=n4SetupAPI.NewProc("SetupDiEnumDeviceInfo")
 n4SetupOpenDriverKey=n4SetupAPI.NewProc("SetupDiOpenDevRegKey")
 n4SetupGetInstance=n4SetupAPI.NewProc("SetupDiGetDeviceInstanceIdW")
 n4SetupDestroy=n4SetupAPI.NewProc("SetupDiDestroyDeviceInfoList")
 n4CfgManager=windows.NewLazySystemDLL("cfgmgr32.dll")
 n4CfgDevNodeStatus=n4CfgManager.NewProc("CM_Get_DevNode_Status")
)

type n4SPDevInfoData struct{
 Size uint32
 ClassGUID windows.GUID
 DevInst uint32
 Reserved uintptr
}

// n4OriginalPnPProbe never exposes raw PnP device IDs, aliases or registry
// values. The hash is for correlating snapshots of a single test device,
// NOT a generation-bound owner capability or permission to uninstall it.
type n4OriginalPnPProbe struct{
 State string
 InstanceSHA256 string
 ProblemCode uint32
}

// n4MaySkipUnkeyedGhost is permitted only while pinning an UNBOUND,
 // independently observed live created Wintun. A WAL-pinned postcrash
 // lookup must NEVER discard an unreadable device instance.
func n4MaySkipUnkeyedGhost(configRet uintptr,alreadyPinned bool)bool{
 return configRet==13 && !alreadyPinned
}

func n4ClassifyCMDevNode(rc uintptr,status uint32)(string,error){
 // CONFIGRET 13 is CR_NO_SUCH_DEVNODE, not proof that SetupAPI,
 // interface aliases, registry DNS settings or a WAL are safe to delete.
 switch{
 case rc==13:return "EXACT_PNP_DEVNODE_GONE_13",nil
 case rc!=0:return "",ErrN4NativeGuard
 case status&0x400!=0:return "EXACT_DEVNODE_PROBLEM",nil
 default:return "EXACT_DEVNODE_NO_PROBLEM",nil
 }
}

// n4RequireOriginalPnPBinding requires the SHA256 sealed in the SAME
// installation/generation WAL to match a fresh Win32 SetupAPI observation.
// Neither a matching digest nor CR_NO_SUCH_DEVNODE authorizes removing the
// device, DNS registry, a route, or a persistent WFP guard.
func n4RequireOriginalPnPBinding(id N4UnifiedIdentity, observed n4OriginalPnPProbe)error{
 if !id.valid() || !n4ValidPinnedPnPHash(id.PnPInstanceSHA256){
  return ErrN4JournalConflict
 }
 if !n4ValidPinnedPnPHash(observed.InstanceSHA256){
  return ErrN4NativeGuard
 }
 if id.PnPInstanceSHA256!=observed.InstanceSHA256 {
  return ErrN4ForeignRoute
 }
 switch observed.State {
 case "EXACT_DEVNODE_NO_PROBLEM","EXACT_DEVNODE_PROBLEM","EXACT_PNP_DEVNODE_GONE_13":
  return nil // read-only identity correlation, NOT cleanup authority
 default:return ErrN4NativeGuard
 }
}

func n4ProbePinnedOriginalWintunPnP(ctx context.Context,id N4UnifiedIdentity)(n4OriginalPnPProbe,error){
 var zero n4OriginalPnPProbe
 if !n4ValidPinnedPnPHash(id.PnPInstanceSHA256){return zero,ErrN4JournalConflict}
 probe,err:=n4ProbeOriginalWintunPnP(ctx,id)
 if err!=nil{return zero,err}
 if err:=n4RequireOriginalPnPBinding(id,probe);err!=nil{return zero,err}
 return probe,nil
}

func n4ProbeOriginalWintunPnP(ctx context.Context,id N4UnifiedIdentity)(n4OriginalPnPProbe,error){
 var zero n4OriginalPnPProbe
 if err:=ctx.Err();err!=nil{return zero,err}
 if !id.valid() || id.WintunGUID=="" || id.AdapterAlias==""{
  return zero,ErrN4JournalConflict
 }
 original,err:=windows.GUIDFromString(id.WintunGUID)
 if err!=nil || original.String()!=id.WintunGUID{return zero,ErrN4JournalConflict}
 if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return zero,err}
 netClass,err:=windows.GUIDFromString("{4d36e972-e325-11ce-bfc1-08002be10318}")
 if err!=nil{return zero,err}
 enumerator,err:=windows.UTF16PtrFromString("SWD\\Wintun")
 if err!=nil{return zero,err}
 if err=n4SetupClassDevs.Find();err!=nil{return zero,err}
 if err=n4SetupEnumDevice.Find();err!=nil{return zero,err}
 if err=n4SetupOpenDriverKey.Find();err!=nil{return zero,err}
 if err=n4SetupGetInstance.Find();err!=nil{return zero,err}
 if err=n4SetupDestroy.Find();err!=nil{return zero,err}
 if err=n4CfgDevNodeStatus.Find();err!=nil{return zero,err}
 handle,_,callErr:=n4SetupClassDevs.Call(uintptr(unsafe.Pointer(&netClass)),uintptr(unsafe.Pointer(enumerator)),0,0)
 if handle==^uintptr(0){return zero,fmt.Errorf("%w: Wintun PnP enumerate: %v",ErrN4NativeGuard,callErr)}
 defer n4SetupDestroy.Call(handle)
 count,found:=0,0
 var out n4OriginalPnPProbe
 for index:=uint32(0);index<256;index++{
  if err:=ctx.Err();err!=nil{return zero,err}
  row:=n4SPDevInfoData{Size:uint32(unsafe.Sizeof(n4SPDevInfoData{}))}
  ok,_,lastErr:=n4SetupEnumDevice.Call(handle,uintptr(index),uintptr(unsafe.Pointer(&row)))
  if ok==0{
   if errors.Is(lastErr,syscall.Errno(259)){
    if err:=n4OriginalWintunPersistedExact(ctx,id);err!=nil{return zero,err}
    if found!=1{return zero,ErrN4NativeGuard}
    return out,nil
   }
   return zero,fmt.Errorf("%w: Wintun PnP enumeration: %v",ErrN4NativeGuard,lastErr)
  }
  count++
  key,_,keyErr:=n4SetupOpenDriverKey.Call(handle,uintptr(unsafe.Pointer(&row)),1,0,2,uintptr(windows.KEY_QUERY_VALUE))
  if key==^uintptr(0){
   // Windows can retain an enumerated SWD\\Wintun ghost after an
   // abruptly terminated prior *test* process. A stale devnode with no
   // driver registry key is not the new LIVE adapter we are pinning.
   // Skip it ONLY if ConfigManager proves CR_NO_SUCH_DEVNODE=13 and the
   // current admission has no pre-existing pinned PnP identity. With an
   // existing WAL pin this is NEVER a skip: postcrash ownership remains
   // fail-closed, even for the same alias or a matching GAA triple.
   var devStatus,devProblem uint32
   devRC,_,_:=n4CfgDevNodeStatus.Call(uintptr(unsafe.Pointer(&devStatus)),
    uintptr(unsafe.Pointer(&devProblem)),uintptr(row.DevInst),0)
   if n4MaySkipUnkeyedGhost(devRC,id.PnPInstanceSHA256!=""){
    continue
   }
   return zero,fmt.Errorf("%w: Wintun driver registry key unavailable (CM status %d): %v",
    ErrN4NativeGuard,devRC,keyErr)
  }
  value,_,valueErr:=registry.Key(key).GetStringValue("NetCfgInstanceId")
  closeErr:=registry.Key(key).Close()
  if valueErr!=nil || closeErr!=nil{
   return zero,fmt.Errorf("%w: Wintun instance registry evidence",ErrN4NativeGuard)
  }
  mapped,parseErr:=windows.GUIDFromString(value)
  if parseErr!=nil{return zero,ErrN4NativeGuard}
  if mapped!=original{continue}
  found++
  if found>1{return zero,ErrN4ForeignRoute}
  // Device instance ID is read for a HASHED receipt, not used for alias
  // reopening, SetupDiRemoveDevice, or any side effect.
  var instance [512]uint16
  var used uint32
  ok,_,lastErr=n4SetupGetInstance.Call(handle,uintptr(unsafe.Pointer(&row)),
   uintptr(unsafe.Pointer(&instance[0])),uintptr(len(instance)),uintptr(unsafe.Pointer(&used)))
  if ok==0 || used==0 || used>uint32(len(instance)){
   return zero,fmt.Errorf("%w: Wintun PnP instance ID: %v",ErrN4NativeGuard,lastErr)
  }
  raw:=windows.UTF16ToString(instance[:])
  if raw==""{return zero,ErrN4NativeGuard}
  sum:=sha256.Sum256([]byte(raw))
  out.InstanceSHA256=hex.EncodeToString(sum[:])
  // Bind every new pinned generation to this exact original device ID.
  // A different PnP instance with the same interface index/alias is foreign.
  if id.PnPInstanceSHA256!="" && id.PnPInstanceSHA256!=out.InstanceSHA256{
   return zero,ErrN4ForeignRoute
  }
  var status,problem uint32
  rc,_,_:=n4CfgDevNodeStatus.Call(uintptr(unsafe.Pointer(&status)),uintptr(unsafe.Pointer(&problem)),uintptr(row.DevInst),0)
  // CR_NO_SUCH_DEVNODE is a distinct *read-only* state, not a
  // native interface or driver removal authorization.
  classified,e:=n4ClassifyCMDevNode(rc,status)
  if e!=nil{return zero,fmt.Errorf("%w: CM_Get_DevNode_Status result=%d",e,rc)}
  out.State=classified
  if rc==0{out.ProblemCode=problem}
 }
 _=count
 return zero,ErrN4NativeGuard // bounded enumeration exhausted
}
