// Adversarial recvmmsg tracepoint fixture, NOT a product or speed benchmark.
// Same TGID/OS thread makes exactly 4 recvmmsg syscalls: 2 target fd=30,
// 2 noise fd=31. A 60ms userspace pause separates the two target calls.
// Local AF_UNIX SOCK_DGRAM socketpairs; no NIC, netns or target product.
#define _GNU_SOURCE
#include <stdio.h>
#include <sys/socket.h>
#include <sys/uio.h>
#include <time.h>
#include <unistd.h>

static int once(int receiver, int sender) {
  char ch='k', buf[8]={0};
  struct iovec iov={.iov_base=buf, .iov_len=sizeof(buf)};
  struct mmsghdr msg={0};
  msg.msg_hdr.msg_iov=&iov;
  msg.msg_hdr.msg_iovlen=1;
  if(send(sender,&ch,1,0)!=1) return 0;
  return recvmmsg(receiver,&msg,1,MSG_DONTWAIT,NULL)==1 && msg.msg_len==1;
}

int main(void) {
  int target[2], noise[2];
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,target)!=0) return 11;
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,noise)!=0) return 12;
  if(dup2(target[0],30)!=30 || dup2(noise[0],31)!=31) return 13;
  close(target[0]);close(noise[0]);
  // Wait for bpftrace BEGIN attachment; fd30/fd31 already established.
  if(getchar()!='G') return 14;
  if(!once(30,target[1])) return 15;
  if(!once(31,noise[1])) return 16;
  struct timespec pause={.tv_sec=0,.tv_nsec=60000000};
  nanosleep(&pause,NULL);
  if(!once(31,noise[1])) return 17;
  if(!once(30,target[1])) return 18;
  close(30);close(31);close(target[1]);close(noise[1]);
  return 0;
}
