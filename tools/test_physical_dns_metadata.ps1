$ErrorActionPreference='Stop'
# Independent fixed Ethernet IPv4 UDP/IPv6 TCP headers, no capture/driver access.
[byte[]]$udp=@(0)*42
$udp[12]=8;$udp[14]=69;$udp[17]=28;$udp[23]=17;$udp[26]=10;$udp[29]=2
$udp[30]=1;$udp[31]=1;$udp[32]=1;$udp[33]=1;$udp[34]=156;$udp[35]=64;$udp[37]=53
if([WBDPhysicalNpcapWatch]::DNSFlowKey($udp,$udp.Length) -ne '10.0.0.2/1.1.1.1/UDP'){throw 'IPv4 UDP DNS metadata wrong'}
$udp[37]=54
if($null -ne [WBDPhysicalNpcapWatch]::DNSFlowKey($udp,$udp.Length)){throw 'Non-DNS port accepted'}
$udp[37]=53;$udp[21]=1
if($null -ne [WBDPhysicalNpcapWatch]::DNSFlowKey($udp,$udp.Length)){throw 'Non-first IPv4 fragment accepted'}
[byte[]]$tcp=@(0)*74
$tcp[12]=134;$tcp[13]=221;$tcp[14]=96;$tcp[19]=20;$tcp[20]=6
$tcp[22]=32;$tcp[23]=1;$tcp[24]=13;$tcp[25]=184;$tcp[37]=1
$tcp[38]=32;$tcp[39]=1;$tcp[40]=13;$tcp[41]=184;$tcp[53]=2;$tcp[55]=53
if([WBDPhysicalNpcapWatch]::DNSFlowKey($tcp,$tcp.Length) -ne '2001:db8::1/2001:db8::2/TCP'){throw 'IPv6 TCP DNS metadata wrong'}
$tcp[20]=0
if($null -ne [WBDPhysicalNpcapWatch]::DNSFlowKey($tcp,$tcp.Length)){throw 'Unsupported IPv6 extension silently decoded'}
if($null -ne [WBDPhysicalNpcapWatch]::DNSFlowKey($udp,13)){throw 'Short frame accepted'}
Write-Output 'DNS_METADATA_PARSER_PASS'
