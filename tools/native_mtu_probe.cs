using System;
using System.Collections.Generic;
using System.Diagnostics;
using System.Net.Sockets;
using System.Threading;

public class WBDNativeMTUCase {
    public int UDPPayload, IPv4Total, Attempts, Sent, ReceivedExact, TimelyReceived, LateExact;
    public int MessageSizeError, OtherSendError, Timeout, BadPayload, Duplicates;
    public bool DontFragment;
}
public class WBDNativeMTUResult {
    public List<WBDNativeMTUCase> Cases=new List<WBDNativeMTUCase>();
    public int RequestedSeconds, InvalidResponses, ReceiveSocketErrors;
    public double ActualSeconds, DrainSeconds, WBDCPUSeconds, HelperCPUSeconds;
    public bool ClientAliveAfter, NoRawPayloadStored=true;
}
public static class WBDNativeMTU {
    private class Pending {public WBDNativeMTUCase Case;public int Length;public double SentAt;public bool Received;}
    private static uint GetSequence(byte[] b) {return (uint)(b[4]|b[5]<<8|b[6]<<16|b[7]<<24);}
    public static WBDNativeMTUResult Run(string target,int port,int seconds,bool maximumUDP,int productPid) {
        if(seconds<1 || seconds>300)throw new ArgumentException("Bounded duration required");
        WBDNativeMTUResult result=new WBDNativeMTUResult();result.RequestedSeconds=seconds;
        Process product=Process.GetProcessById(productPid),helper=Process.GetCurrentProcess();
        double cpuBefore=product.TotalProcessorTime.TotalSeconds,helperBefore=helper.TotalProcessorTime.TotalSeconds;
        Stopwatch clock=Stopwatch.StartNew();
        Dictionary<uint,Pending> pending=new Dictionary<uint,Pending>();
        Dictionary<string,WBDNativeMTUCase> cases=new Dictionary<string,WBDNativeMTUCase>();
        uint sequence=0;byte[] receive=new byte[65535];
        int[] sizes=maximumUDP?new int[]{8972,8973,65507,65508}:new int[]{1371,1372,1373,1472,1972,4068,8972};
        using(Socket sock=new Socket(AddressFamily.InterNetwork,SocketType.Dgram,ProtocolType.Udp)) {
            sock.ReceiveBufferSize=2*1024*1024;sock.Connect(target,port);
            Action<int> receiveOne=delegate(int timeout) {
                sock.ReceiveTimeout=Math.Max(1,timeout);int n;
                try{n=sock.Receive(receive);}catch(SocketException error){if(error.SocketErrorCode!=SocketError.TimedOut)result.ReceiveSocketErrors++;return;}
                if(n<8 || receive[0]!='P' || receive[1]!='7' || receive[2]!='M' || receive[3]!='1'){result.InvalidResponses++;return;}
                Pending p;if(!pending.TryGetValue(GetSequence(receive),out p)){result.InvalidResponses++;return;}
                bool valid=n==p.Length;
                for(int i=8;i<n && valid;i++)if(receive[i]!=(byte)((i-8)%256))valid=false;
                if(!valid){p.Case.BadPayload++;return;}
                if(p.Received){p.Case.Duplicates++;return;}
                p.Received=true;p.Case.ReceivedExact++;
                if(clock.Elapsed.TotalSeconds-p.SentAt<=1)p.Case.TimelyReceived++;else p.Case.LateExact++;
            };
            Action<int,bool> probe=delegate(int length,bool df) {
                if(sequence>=8192)throw new InvalidOperationException("Bounded request inventory exceeded");
                string key=length.ToString()+"/"+df.ToString();WBDNativeMTUCase c;
                if(!cases.TryGetValue(key,out c)){c=new WBDNativeMTUCase{UDPPayload=length,IPv4Total=length+28,DontFragment=df};cases[key]=c;result.Cases.Add(c);}
                c.Attempts++;uint id=sequence++;byte[] b=new byte[length];b[0]=(byte)'P';b[1]=(byte)'7';b[2]=(byte)'M';b[3]=(byte)'1';
                for(int i=0;i<4;i++)b[4+i]=(byte)(id>>(i*8));
                for(int i=8;i<b.Length;i++)b[i]=(byte)((i-8)%256);
                sock.DontFragment=df;double started=clock.Elapsed.TotalSeconds;
                Pending p=null;
                try{sock.Send(b);c.Sent++;p=new Pending{Case=c,Length=length,SentAt=started};pending[id]=p;}
                catch(SocketException error){if(error.SocketErrorCode==SocketError.MessageSize)c.MessageSizeError++;else c.OtherSendError++;}
                if(p!=null){
                    while(!p.Received && clock.Elapsed.TotalSeconds-started<1)receiveOne((int)Math.Ceiling((1-(clock.Elapsed.TotalSeconds-started))*1000));
                    if(!p.Received)c.Timeout++;
                }
                // Qualification probes, not a throughput load. Fixed maximum
                // ten attempts/second includes rejected sends in the budget.
                int wait=(int)Math.Ceiling((.1-(clock.Elapsed.TotalSeconds-started))*1000);
                if(wait>0)Thread.Sleep(wait);
            };
            while(clock.Elapsed.TotalSeconds<seconds){
                foreach(bool df in new bool[]{false,true})foreach(int size in sizes){
                    if(clock.Elapsed.TotalSeconds>=seconds)break;
                    probe(size,df);
                    if(clock.Elapsed.TotalSeconds<seconds)probe(96,false);
                }
            }
            result.ActualSeconds=clock.Elapsed.TotalSeconds;
            double drainStart=clock.Elapsed.TotalSeconds;
            while(clock.Elapsed.TotalSeconds-drainStart<3)receiveOne(100);
            result.DrainSeconds=clock.Elapsed.TotalSeconds-drainStart;
            sock.Send(System.Text.Encoding.ASCII.GetBytes("P7M-DONE"));
        }
        product.Refresh();helper.Refresh();result.ClientAliveAfter=!product.HasExited;
        if(result.ClientAliveAfter)result.WBDCPUSeconds=product.TotalProcessorTime.TotalSeconds-cpuBefore;
        result.HelperCPUSeconds=helper.TotalProcessorTime.TotalSeconds-helperBefore;
        return result;
    }
}
