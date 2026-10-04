using System;
using System.Collections.Generic;
using System.IO;
using System.Diagnostics;
using System.Globalization;
using System.Net;
using System.Net.Sockets;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

public class WBDPhysicalUDPResult {
    public long TxPackets, TxBytes, RxPackets, RxBytes, DuplicatePackets, BadPayload, ReorderedPackets;
    public double MaxSendLagMs, ProbeP95Ms, ProbeP99Ms, WBDCPUSeconds, HelperCPUSeconds;
    public int ProbeCount, ProbeSent, Seconds;
    public double RequestedMbps;
    public volatile string ServerJSON;
    public bool SummaryReceived;
    public bool NoPcap = true;
    public int LiveSnapshotWriteErrors;
    public List<WBDPhysicalUDPInterval> Intervals = new List<WBDPhysicalUDPInterval>();
}
public class WBDPhysicalUDPInterval {
    public double ElapsedSeconds, WBDCPUSeconds;
    public long TxBytes, RxBytes;
}
public static class WBDPhysicalUDP {
    [DllImport("winmm.dll")] static extern uint timeBeginPeriod(uint period);
    [DllImport("winmm.dll")] static extern uint timeEndPeriod(uint period);
    static int[] sizes = new int[] {96,256,512,1000,1372};
    static void Put32(byte[] b, int at, uint n) {for(int i=0;i<4;i++) b[at+i]=(byte)(n >> (24-8*i));}
    static uint Get32(byte[] b,int at) {return (uint)(b[at]<<24 | b[at+1]<<16 | b[at+2]<<8 | b[at+3]);}
    public static WBDPhysicalUDPResult Run(string target,int port,double mbps,int seconds,uint seed,int wbdPid,string livePath="") {
        if(seconds<1 || seconds>600 || mbps<=0 || mbps>40 || Math.Ceiling(mbps*1000000/8*seconds/647.2)+5>=1000000) throw new ArgumentException("Out of bounded test duration/bitmap range");
        WBDPhysicalUDPResult result=new WBDPhysicalUDPResult(); result.Seconds=seconds;result.RequestedMbps=mbps;
        Process app=Process.GetProcessById(wbdPid); double appBefore=app.TotalProcessorTime.TotalSeconds;
        double helperBefore=Process.GetCurrentProcess().TotalProcessorTime.TotalSeconds;
        using(Socket sock=new Socket(AddressFamily.InterNetwork,SocketType.Dgram,ProtocolType.Udp)) {
            sock.ReceiveBufferSize=2*1024*1024; sock.ReceiveTimeout=1000; sock.Connect(target,port);
            byte[] greeting=Encoding.ASCII.GetBytes("P7H {\"Mbps\":"+mbps.ToString(CultureInfo.InvariantCulture)+",\"Seconds\":"+seconds+",\"Seed\":"+seed+"}");
            byte[] receive=new byte[4096]; bool admitted=false;
            for(int attempt=0;attempt<8 && !admitted;attempt++) {
                sock.Send(greeting);
                try {int n=sock.Receive(receive);admitted=n==3 && Encoding.ASCII.GetString(receive,0,n)=="P7A";} catch(SocketException) {}
            }
            if(!admitted) throw new Exception("Controlled UDP hello timeout");
            bool[] seen=new bool[1000000]; List<double> rtts=new List<double>();
            int done=0; long maxSequence=-1,lastProbeAckTick=Stopwatch.GetTimestamp();
            Thread rx=new Thread(delegate() {
                while(Interlocked.CompareExchange(ref done,0,0)==0) {
                    int n; try {n=sock.Receive(receive);} catch(SocketException) {continue;}
                    if(n>=4 && receive[0]=='P' && receive[1]=='7' && receive[2]=='R' && receive[3]==' ') {
                        result.ServerJSON=Encoding.UTF8.GetString(receive,4,n-4); continue;
                    }
                    if(n==12 && receive[0]=='P' && receive[1]=='7' && receive[2]=='P') {
                        long sent=BitConverter.ToInt64(receive,4);
                        Interlocked.Exchange(ref lastProbeAckTick,Stopwatch.GetTimestamp());
                        rtts.Add((Stopwatch.GetTimestamp()-sent)*1000.0/Stopwatch.Frequency);continue;
                    }
                    if(n<16 || receive[0]!='P' || receive[1]!='7' || receive[2]!='D' || receive[3]!='1') {result.BadPayload++;continue;}
                    uint sequence=Get32(receive,8);int declared=(receive[12]<<8)|receive[13]; bool valid=true;
                    if(Get32(receive,4)!=seed || sequence>=seen.Length || declared!=n || receive[14]!=1) valid=false;
                    for(int i=16;i<n && valid;i++) if(receive[i]!=(byte)((i-16)%256)) valid=false;
                    if(!valid) {result.BadPayload++;continue;}
                    if(seen[sequence]) {result.DuplicatePackets++;continue;}
                    seen[sequence]=true;
                    if(sequence<maxSequence) result.ReorderedPackets++;
                    maxSequence=Math.Max(maxSequence,sequence); result.RxPackets++;result.RxBytes+=n;
                }
            });
            rx.IsBackground=true;rx.Start();timeBeginPeriod(1);
            try {
                sock.Send(Encoding.ASCII.GetBytes("P7G"));
                Dictionary<int,byte[]> buffers=new Dictionary<int,byte[]>();
                foreach(int size in sizes) {byte[] b=new byte[size];b[0]=(byte)'P';b[1]=(byte)'7';b[2]=(byte)'D';b[3]=(byte)'1';Put32(b,4,seed);b[12]=(byte)(size>>8);b[13]=(byte)size;for(int i=16;i<size;i++) b[i]=(byte)((i-16)%256);buffers[size]=b;}
                Stopwatch watch=Stopwatch.StartNew(); double rate=mbps*1000000/8;uint sequence=0;double nextProbe=0,nextReport=1;
                byte[] probe=new byte[12];probe[0]=(byte)'P';probe[1]=(byte)'7';probe[2]=(byte)'P';
                while(watch.Elapsed.TotalSeconds<seconds) {
                    double elapsed=watch.Elapsed.TotalSeconds, budget=elapsed*rate;int batch=0;
                    while(result.TxBytes+sizes[sequence%sizes.Length]<=budget && batch<32) {
                        int size=sizes[sequence%sizes.Length];byte[] b=buffers[size];Put32(b,8,sequence);
                        sock.Send(b);result.TxBytes+=size;result.TxPackets++;sequence++;batch++;
                        result.MaxSendLagMs=Math.Max(result.MaxSendLagMs,(elapsed-result.TxBytes/rate)*1000);
                    }
                    if(elapsed>=nextProbe) {Array.Copy(BitConverter.GetBytes(Stopwatch.GetTimestamp()),0,probe,4,8);sock.Send(probe);result.ProbeSent++;nextProbe=elapsed+.1;}
                    if(elapsed>=nextReport) {
                        app.Refresh();long rxBytes=Interlocked.Read(ref result.RxBytes);
                        result.Intervals.Add(new WBDPhysicalUDPInterval {ElapsedSeconds=elapsed,TxBytes=result.TxBytes,RxBytes=rxBytes,WBDCPUSeconds=app.TotalProcessorTime.TotalSeconds-appBefore});nextReport=elapsed+1;
                        if(livePath!="") {
                            string text="{\"ElapsedSeconds\":"+elapsed.ToString("R",CultureInfo.InvariantCulture)+",\"TxBytes\":"+result.TxBytes+",\"RxBytes\":"+rxBytes+",\"SecondsSinceProbeReply\":"+((Stopwatch.GetTimestamp()-Interlocked.Read(ref lastProbeAckTick))/(double)Stopwatch.Frequency).ToString("R",CultureInfo.InvariantCulture)+"}";
                            // Diagnostic readers must never interrupt the offered workload.
                            try {
                                File.WriteAllText(livePath+".next",text,new UTF8Encoding(false));
                                if(File.Exists(livePath))File.Replace(livePath+".next",livePath,null);else File.Move(livePath+".next",livePath);
                            }catch(IOException){result.LiveSnapshotWriteErrors++;}
                        }
                    }
                    Thread.Sleep(1);
                }
                Thread.Sleep(3200);
                for(int i=0;i<30 && result.ServerJSON==null;i++) {sock.Send(Encoding.ASCII.GetBytes("P7RESULT"));Thread.Sleep(200);}
                result.SummaryReceived=result.ServerJSON!=null;
                sock.Send(Encoding.ASCII.GetBytes("P7DONE"));
            } finally {timeEndPeriod(1);Interlocked.Exchange(ref done,1);rx.Join(2000);}
            rtts.Sort();result.ProbeCount=rtts.Count;
            if(rtts.Count>0) {result.ProbeP95Ms=rtts[(int)Math.Ceiling(rtts.Count*.95)-1];result.ProbeP99Ms=rtts[(int)Math.Ceiling(rtts.Count*.99)-1];}
        }
        app.Refresh();result.WBDCPUSeconds=app.TotalProcessorTime.TotalSeconds-appBefore;
        result.HelperCPUSeconds=Process.GetCurrentProcess().TotalProcessorTime.TotalSeconds-helperBefore;
        return result;
    }
}
