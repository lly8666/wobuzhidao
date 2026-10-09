// Standalone non-product E4 recvmmsg calibration fixture.
// One process, 1 OS thread, 4,000 target recvmmsg/s for exactly 12s.
// 48,000 target + 48,000 distractor syscalls, AF_UNIX only, no NIC.
// Outputs one aggregated JSON line. No payload, FD or PID in report.
#define _GNU_SOURCE
#include <errno.h>
#include <inttypes.h>
#include <stdint.h>
#include <stdio.h>
#include <stdlib.h>
#include <sys/resource.h>
#include <sys/socket.h>
#include <sys/uio.h>
#include <time.h>
#include <unistd.h>

#define RATE 4000u
#define SECONDS 12u
#define COUNT (RATE * SECONDS)
static uint64_t samples[COUNT];

static uint64_t ns(clockid_t clock_id) {
  struct timespec ts={0};
  if(clock_gettime(clock_id,&ts)!=0) exit(31);
  return (uint64_t)ts.tv_sec*1000000000ull+(uint64_t)ts.tv_nsec;
}
static int cmp_u64(const void *a,const void *b) {
  uint64_t x=*(const uint64_t *)a,y=*(const uint64_t *)b;
  return (x>y)-(x<y);
}
static int transfer(int recv_fd,int send_fd,uint64_t *duration) {
  char value='a', received[8]={0};
  struct iovec iov={.iov_base=received,.iov_len=sizeof(received)};
  struct mmsghdr h={0};
  h.msg_hdr.msg_iov=&iov;
  h.msg_hdr.msg_iovlen=1;
  if(send(send_fd,&value,1,MSG_DONTWAIT)!=1) return 0;
  uint64_t enter=ns(CLOCK_MONOTONIC);
  int received_count=recvmmsg(recv_fd,&h,1,MSG_DONTWAIT,NULL);
  *duration=ns(CLOCK_MONOTONIC)-enter;
  return received_count==1 && h.msg_len==1;
}
static uint64_t quantile(unsigned permille) {
  uint64_t idx=((uint64_t)COUNT*permille+999)/1000;
  if(idx==0)idx=1;
  return samples[idx-1];
}
int main(void) {
  int a[2],b[2];
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,a)!=0) return 11;
  if(socketpair(AF_UNIX,SOCK_DGRAM,0,b)!=0) return 12;
  if(dup2(a[0],30)!=30 || dup2(b[0],31)!=31) return 13;
  close(a[0]);close(b[0]);
  // The Python parent releases exactly one mode after any BPF attachment.
  if(getchar()!='G') return 14;
  uint64_t first=ns(CLOCK_MONOTONIC),last=first,missed_deadlines=0;
  uint64_t wait_max_ns=0;
  for(unsigned i=0;i<COUNT;i++) {
    uint64_t scheduled=first+(uint64_t)i*1000000000ull/RATE;
    struct timespec wake={(time_t)(scheduled/1000000000ull),
                          (long)(scheduled%1000000000ull)};
    int code;
    do {
      code=clock_nanosleep(CLOCK_MONOTONIC,TIMER_ABSTIME,&wake,NULL);
    } while(code==EINTR);
    if(code!=0)return 20;
    uint64_t now=ns(CLOCK_MONOTONIC);
    if(now>scheduled+250000ull) missed_deadlines++;
    if(now>scheduled && now-scheduled>wait_max_ns)wait_max_ns=now-scheduled;
    uint64_t target=0,decoy=0;
    if(!transfer(30,a[1],&target))return 21;
    if(!transfer(31,b[1],&decoy))return 22;
    samples[i]=target;
    last=ns(CLOCK_MONOTONIC);
  }
  uint64_t elapsed=last-first;
  qsort(samples,COUNT,sizeof(samples[0]),cmp_u64);
  struct rusage usage={0};
  if(getrusage(RUSAGE_SELF,&usage)!=0)return 25;
  uint64_t user_us=(uint64_t)usage.ru_utime.tv_sec*1000000ull+usage.ru_utime.tv_usec;
  uint64_t system_us=(uint64_t)usage.ru_stime.tv_sec*1000000ull+usage.ru_stime.tv_usec;
  printf("{\"schema\":\"wbd-e4-calibration-fixture/v1\","
         "\"target_calls\":%u,\"decoy_calls\":%u,\"target_rate_hz\":%u,"
         "\"elapsed_ns\":%" PRIu64 ",\"recv_p99_ns\":%" PRIu64 ","
         "\"recv_p999_ns\":%" PRIu64 ",\"recv_max_ns\":%" PRIu64 ","
         "\"late_over_250us\":%" PRIu64 ",\"maximum_lateness_ns\":%" PRIu64 ","
         "\"cpu_user_us\":%" PRIu64 ",\"cpu_system_us\":%" PRIu64 ","
         "\"maxrss_kib\":%ld}\n",
         COUNT,COUNT,RATE,elapsed,quantile(990),quantile(999),samples[COUNT-1],
         missed_deadlines,wait_max_ns,user_us,system_us,usage.ru_maxrss);
  fflush(stdout);
  close(30);close(31);close(a[1]);close(b[1]);
  return 0;
}
