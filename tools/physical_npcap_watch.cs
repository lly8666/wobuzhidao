using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Globalization;
using System.IO;
using System.Runtime.InteropServices;
using System.Text;

// Independent qualification capture handle; never sends or retains payloads.
public static class WBDPhysicalNpcapWatch {
    [DllImport("kernel32.dll",CharSet=CharSet.Unicode)] static extern IntPtr LoadLibrary(string path);
    [DllImport("kernel32.dll",CharSet=CharSet.Ansi)] static extern IntPtr GetProcAddress(IntPtr module,string name);
    [DllImport("kernel32.dll")] static extern bool FreeLibrary(IntPtr module);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate IntPtr Open(string device,int snap,int promisc,int timeout,StringBuilder error);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate int Next(IntPtr handle,out IntPtr header,out IntPtr data);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate int Compile(IntPtr handle,ref Program program,string filter,int optimize,uint mask);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate int SetFilter(IntPtr handle,ref Program program);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate void FreeCode(ref Program program);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate void Close(IntPtr handle);
    [UnmanagedFunctionPointer(CallingConvention.Cdecl)] delegate int Stats(IntPtr handle,out PcapStats stats);
    [StructLayout(LayoutKind.Sequential)] struct Header {public int Seconds,Microseconds;public uint Caplen,Length;}
    [StructLayout(LayoutKind.Sequential)] struct Program {public uint Length;public IntPtr Instructions;}
    // Windows pcap_stat has six uint fields; see Npcap API and libpcap pcap.h.
    [StructLayout(LayoutKind.Sequential)] struct PcapStats {public uint Received,Dropped,InterfaceDropped,Captured,Sent,NetworkDropped;}
    class Flow {public long Packets,PayloadPackets,PayloadBytes,Syn,Fin,Rst;public uint Seq,Ack;}
    static T Proc<T>(IntPtr dll,string name) {IntPtr p=GetProcAddress(dll,name);if(p==IntPtr.Zero)throw new Exception("Npcap export missing: "+name);return (T)(object)Marshal.GetDelegateForFunctionPointer(p,typeof(T));}
    static uint U32(byte[] b,int p) {return (uint)(b[p]<<24|b[p+1]<<16|b[p+2]<<8|b[p+3]);}
    static int U16(byte[] b,int p) {return b[p]<<8|b[p+1];}
    // Pure metadata parser for an independent DNS leak observer. Unsupported
    // headers are counted as unparsed, never silently called zero leakage.
    public static string DNSFlowKey(byte[] frame,int n) {
        if(frame==null||n<14||n>frame.Length)return null;
        int ip=14,type=U16(frame,12);
        for(int tag=0;(type==0x8100||type==0x88a8)&&tag<2;tag++){if(n<ip+4)return null;type=U16(frame,ip+2);ip+=4;}
        int transport,protocol,declaredTransport;byte[] src,dst;
        if(type==0x0800) {
            if(n<ip+20||frame[ip]>>4!=4)return null;
            int ihl=(frame[ip]&15)*4;if(ihl<20||n<ip+ihl||(U16(frame,ip+6)&0x1fff)!=0)return null;
            transport=ip+ihl;declaredTransport=U16(frame,ip+2)-ihl;protocol=frame[ip+9];src=new byte[4];dst=new byte[4];
            Array.Copy(frame,ip+12,src,0,4);Array.Copy(frame,ip+16,dst,0,4);
        }else if(type==0x86dd) {
            if(n<ip+40||frame[ip]>>4!=6)return null;
            transport=ip+40;declaredTransport=U16(frame,ip+4);protocol=frame[ip+6];src=new byte[16];dst=new byte[16];
            Array.Copy(frame,ip+8,src,0,16);Array.Copy(frame,ip+24,dst,0,16);
        }else return null;
        if((protocol!=6&&protocol!=17)||n<transport+(protocol==6?20:8))return null;
        if(declaredTransport<(protocol==6?20:8))return null;
        if(U16(frame,transport)!=53&&U16(frame,transport+2)!=53)return null;
        return new System.Net.IPAddress(src).ToString()+"/"+new System.Net.IPAddress(dst).ToString()+"/"+(protocol==6?"TCP":"UDP");
    }
    public static void Run(string device,string localIP,string peerIP,int seconds,string output) {
        RunCapture(device,localIP,peerIP,seconds,output,false);
    }
    public static void RunDNS(string device,int seconds,string output) {
        RunCapture(device,"0.0.0.0","0.0.0.0",seconds,output,true);
    }
    static void DNSReport(StreamWriter writer,Dictionary<string,Flow> flows,Stopwatch watch,PcapStats ps,int code,long frames,long unparsed,bool final) {
        StringBuilder line=new StringBuilder();
        line.Append("{\"ElapsedSeconds\":").Append(watch.Elapsed.TotalSeconds.ToString("R",CultureInfo.InvariantCulture)).Append(",\"UnixMS\":").Append((DateTime.UtcNow.Ticks-621355968000000000L)/10000).Append(",\"ObserverReceived\":").Append(ps.Received).Append(",\"ObserverDropped\":").Append(ps.Dropped).Append(",\"StatsReturnCode\":").Append(code).Append(",\"PayloadStored\":false,\"Final\":").Append(final?"true":"false").Append(",\"DNSFrames\":").Append(frames).Append(",\"UnparsedFrames\":").Append(unparsed).Append(",\"DNSFlows\":{");
        bool first=true;foreach(KeyValuePair<string,Flow> entry in flows){if(!first)line.Append(',');first=false;line.Append('"').Append(entry.Key).Append("\":").Append(entry.Value.Packets);}
        line.Append("}}");writer.WriteLine(line.ToString());writer.Flush();
    }
    static void RunCapture(string device,string localIP,string peerIP,int seconds,string output,bool dnsOnly) {
        if(seconds<1||seconds>660)throw new ArgumentException("Duration out of bound");
        byte[] local=System.Net.IPAddress.Parse(localIP).GetAddressBytes(),peer=System.Net.IPAddress.Parse(peerIP).GetAddressBytes();
        if(local.Length!=4||peer.Length!=4)throw new ArgumentException("IPv4 required");
        string dir=Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.Windows),"System32","Npcap");
        IntPtr packet=LoadLibrary(Path.Combine(dir,"Packet.dll")),dll=IntPtr.Zero,handle=IntPtr.Zero;
        try {
            dll=LoadLibrary(Path.Combine(dir,"wpcap.dll"));if(dll==IntPtr.Zero)throw new Exception("Npcap DLL load failed");
            Open open=Proc<Open>(dll,"pcap_open_live");Next next=Proc<Next>(dll,"pcap_next_ex");Close close=Proc<Close>(dll,"pcap_close");Stats stats=Proc<Stats>(dll,"pcap_stats");
            StringBuilder error=new StringBuilder(256);handle=open(device,160,0,50,error);if(handle==IntPtr.Zero)throw new Exception("Capture open failed: "+error);
            Compile compile=Proc<Compile>(dll,"pcap_compile");SetFilter set=Proc<SetFilter>(dll,"pcap_setfilter");FreeCode free=Proc<FreeCode>(dll,"pcap_freecode");
            Program program=new Program();string filter=dnsOnly?"(ip or ip6) and (udp or tcp) and port 53":"tcp and host "+localIP+" and host "+peerIP+" and port 443";
            if(compile(handle,ref program,filter,1,0xffffffff)!=0)throw new Exception("Filter compile failed");
            try {if(set(handle,ref program)!=0)throw new Exception("Filter install failed");}finally {free(ref program);}
            Dictionary<string,Flow> flows=new Dictionary<string,Flow>();byte[] frame=new byte[160];Stopwatch watch=Stopwatch.StartNew();double nextReport=1;
            long dnsFrames=0,dnsUnparsed=0;
            using(StreamWriter writer=new StreamWriter(output,false,new UTF8Encoding(false))) {
                while(watch.Elapsed.TotalSeconds<seconds) {
                    if(watch.Elapsed.TotalSeconds>=nextReport) {
                        PcapStats ps;int code=stats(handle,out ps);StringBuilder line=new StringBuilder();
                        if(dnsOnly) {
                            DNSReport(writer,flows,watch,ps,code,dnsFrames,dnsUnparsed,false);nextReport=watch.Elapsed.TotalSeconds+1;
                        }else {
                        line.Append("{\"ElapsedSeconds\":").Append(watch.Elapsed.TotalSeconds.ToString("R",CultureInfo.InvariantCulture)).Append(",\"ObserverReceived\":").Append(ps.Received).Append(",\"ObserverDropped\":").Append(ps.Dropped).Append(",\"StatsReturnCode\":").Append(code).Append(",\"PayloadStored\":false,\"Flows\":{");
                        bool first=true;foreach(KeyValuePair<string,Flow> entry in flows){if(!first)line.Append(',');first=false;Flow v=entry.Value;line.Append('"').Append(entry.Key).Append("\":{\"Packets\":").Append(v.Packets).Append(",\"PayloadPackets\":").Append(v.PayloadPackets).Append(",\"PayloadBytes\":").Append(v.PayloadBytes).Append(",\"Syn\":").Append(v.Syn).Append(",\"Fin\":").Append(v.Fin).Append(",\"Rst\":").Append(v.Rst).Append(",\"LastSeq\":").Append(v.Seq).Append(",\"LastAck\":").Append(v.Ack).Append('}');}
                        line.Append("}}");writer.WriteLine(line.ToString());writer.Flush();nextReport=watch.Elapsed.TotalSeconds+1;
                        }
                    }
                    IntPtr header,data;int ret=next(handle,out header,out data);if(ret==0)continue;if(ret!=1)throw new Exception("Capture read failed: "+ret);
                    Header h=(Header)Marshal.PtrToStructure(header,typeof(Header));int n=(int)Math.Min(h.Caplen,160);
                    if(dnsOnly) {
                        if(++dnsFrames>8192)throw new Exception("DNS observer bounded inventory exceeded");
                        Marshal.Copy(data,frame,0,n);string key=DNSFlowKey(frame,n);
                        if(key==null){dnsUnparsed++;continue;}
                        Flow dnsFlow;if(!flows.TryGetValue(key,out dnsFlow)){if(flows.Count>=128)throw new Exception("DNS observer flow bound exceeded");dnsFlow=new Flow();flows[key]=dnsFlow;}
                        dnsFlow.Packets++;continue;
                    }
                    if(n<54)continue;Marshal.Copy(data,frame,0,n);
                    int ip=14,type=U16(frame,12);for(int tag=0;(type==0x8100||type==0x88a8)&&tag<2;tag++){if(n<ip+4)break;type=U16(frame,ip+2);ip+=4;}
                    if(type!=0x0800||n<ip+40||frame[ip]>>4!=4||frame[ip+9]!=6||(U16(frame,ip+6)&0x3fff)!=0)continue;
                    bool incoming=true,outgoing=true;for(int i=0;i<4;i++){incoming&=frame[ip+12+i]==peer[i]&&frame[ip+16+i]==local[i];outgoing&=frame[ip+12+i]==local[i]&&frame[ip+16+i]==peer[i];}
                    if(!incoming&&!outgoing)continue;int ihl=(frame[ip]&15)*4,tcp=ip+ihl;if(ihl<20||n<tcp+20)continue;
                    int port=U16(frame,tcp+(incoming?2:0));if(U16(frame,tcp+(incoming?0:2))!=443||port<40000||port>=41024)continue;
                    int tcpLen=(frame[tcp+12]>>4)*4,payload=U16(frame,ip+2)-ihl-tcpLen;if(tcpLen<20||payload<0)continue;
                    string key=(incoming?"S2C/":"C2S/")+port;Flow flow;if(!flows.TryGetValue(key,out flow)){flow=new Flow();flows[key]=flow;}
                    flow.Packets++;if(payload>0)flow.PayloadPackets++;flow.PayloadBytes+=payload;flow.Syn+=(frame[tcp+13]&2)!=0?1:0;flow.Fin+=(frame[tcp+13]&1)!=0?1:0;flow.Rst+=(frame[tcp+13]&4)!=0?1:0;flow.Seq=U32(frame,tcp+4);flow.Ack=U32(frame,tcp+8);
                }
                if(dnsOnly){PcapStats ps;int code=stats(handle,out ps);DNSReport(writer,flows,watch,ps,code,dnsFrames,dnsUnparsed,true);}
            }
            close(handle);handle=IntPtr.Zero;
        }finally {if(handle!=IntPtr.Zero&&dll!=IntPtr.Zero)Proc<Close>(dll,"pcap_close")(handle);if(dll!=IntPtr.Zero)FreeLibrary(dll);if(packet!=IntPtr.Zero)FreeLibrary(packet);}
    }
}
