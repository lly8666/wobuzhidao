// Non-performance eBPF event-integrity fixture. Exact 256 recvmmsg
// syscalls on target fd30 and 256 on distractor fd31, same TGID.
// Only local AF_UNIX SOCK_DGRAM socketpairs; no NIC, product or benchmarking.
#define _GNU_SOURCE
#include <stdio.h>
#include <sys/socket.h>
#include <sys/uio.h>
#include <unistd.h>

static int transfer(int readfd,int sendfd) {
  char x='x', y[4]={0};
  struct iovec iov={.iov_base=y,.iov_len=sizeof(y)};
  struct mmsghdr msg={0};
  msg.msg_hdr.msg_iov=&iov;
  msg.msg_hdr.msg_iovlen=1;
  if(send(sendfd,&x,1,MSG_DONTWAIT)!=1) return 0;
  return recvmmsg(readfd,&msg,1,MSG_DONTWAIT,NULL)==1 && msg.msg_len==1;
}
int main(void) {
  int target[2],noise[2];
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,target)!=0) return 11;
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,noise)!=0) return 12;
  if(dup2(target[0],30)!=30 || dup2(noise[0],31)!=31) return 13;
  close(target[0]);close(noise[0]);
  if(getchar()!='G') return 14;
  for(int i=0;i<256;i++) {
    if(!transfer(30,target[1])) return 15;
    if(!transfer(31,noise[1])) return 16;
  }
  close(30);close(31);close(target[1]);close(noise[1]);
  return 0;
}
