#include <wolfssl/options.h>
#include <wolfssl/ssl.h>
#include <arpa/inet.h>
#include <errno.h>
#include <fcntl.h>
#include <netinet/in.h>
#include <poll.h>
#include <signal.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <time.h>
#include <unistd.h>

static volatile sig_atomic_t g_stop = 0;
static void on_signal(int sig) { (void)sig; g_stop = 1; }
static void die(const char* what) { perror(what); exit(2); }
static void ssl_log(const char* what, WOLFSSL* ssl, int ret) {
    int err=wolfSSL_get_error(ssl,ret); char b[160];
    wolfSSL_ERR_error_string((unsigned long)err,b);
    fprintf(stderr,"%s ret=%d err=%d msg=%s\n",what,ret,err,b);
}
static struct sockaddr_in addr4(const char* ip,int port){
    struct sockaddr_in a; memset(&a,0,sizeof(a)); a.sin_family=AF_INET; a.sin_port=htons((unsigned short)port);
    if(inet_pton(AF_INET,ip,&a.sin_addr)!=1){fprintf(stderr,"bad IPv4: %s\n",ip);exit(2);} return a;
}
static void timeout_fd(int fd){struct timeval tv={5,0};setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&tv,sizeof(tv));setsockopt(fd,SOL_SOCKET,SO_SNDTIMEO,&tv,sizeof(tv));}
static void nonblock(int fd){int f=fcntl(fd,F_GETFL,0);if(f<0||fcntl(fd,F_SETFL,f|O_NONBLOCK)<0)die("fcntl");}
static int write_record(WOLFSSL* ssl,const unsigned char* buf,int n){
    for(;;){
        int r=wolfSSL_write(ssl,buf,n);
        if(r==n)return r;
        int e=wolfSSL_get_error(ssl,r);
        if(e==WOLFSSL_ERROR_WANT_WRITE||e==WOLFSSL_ERROR_WANT_READ){struct pollfd p={wolfSSL_get_fd(ssl),(e==WOLFSSL_ERROR_WANT_WRITE)?POLLOUT:POLLIN,0};poll(&p,1,1000);continue;}
        ssl_log("wolfSSL_write",ssl,r);return -1;
    }
}

typedef struct { unsigned long plain_in,plain_out,dtls_write_calls,dtls_read_records; unsigned long long plain_in_bytes,plain_out_bytes; } Stats;

static int relay_loop(const char* role,WOLFSSL* ssl,int transport,int plain,int plain_is_client){
    unsigned char buf[65535]; struct sockaddr_in app_peer; socklen_t app_len=sizeof(app_peer); int have_peer=0; Stats st={0};
    nonblock(transport); nonblock(plain); wolfSSL_set_using_nonblock(ssl, 1);
    struct pollfd pf[2]={{plain,POLLIN,0},{transport,POLLIN,0}};
    while(!g_stop){
        int pr=poll(pf,2,200); if(pr<0){if(errno==EINTR)continue;die("poll");}
        if(pf[0].revents&POLLIN){
            ssize_t n;
            if(plain_is_client){n=recvfrom(plain,buf,sizeof(buf),0,(struct sockaddr*)&app_peer,&app_len);if(n>0)have_peer=1;}
            else n=recv(plain,buf,sizeof(buf),0);
            if(n>0){
                st.plain_in++;st.plain_in_bytes+=(unsigned long long)n;st.dtls_write_calls++;
                fprintf(stderr,"WRITE role=%s datagram=%lu bytes=%zd\n",role,st.plain_in,n);
                if(write_record(ssl,buf,(int)n)<0)return 5;
            }
        }
        if(pf[1].revents&POLLIN){
            for(;;){
                int r=wolfSSL_read(ssl,buf,sizeof(buf));
                if(r>0){
                    st.dtls_read_records++;st.plain_out++;st.plain_out_bytes+=(unsigned long long)r;
                    fprintf(stderr,"READ role=%s record=%lu bytes=%d\n",role,st.dtls_read_records,r);
                    ssize_t s;
                    if(plain_is_client){if(!have_peer){fprintf(stderr,"no client plaintext peer\n");return 6;}s=sendto(plain,buf,(size_t)r,0,(struct sockaddr*)&app_peer,app_len);}
                    else s=send(plain,buf,(size_t)r,0);
                    if(s!=r){perror("plain send");return 6;}
                    continue;
                }
                int e=wolfSSL_get_error(ssl,r);
                if(e==WOLFSSL_ERROR_WANT_READ||e==WOLFSSL_ERROR_WANT_WRITE)break;
                if(r==0)break;
                ssl_log("wolfSSL_read",ssl,r);return 5;
            }
        }
    }
    fprintf(stderr,"STATS role=%s plain_in=%lu plain_out=%lu dtls_writes=%lu dtls_records=%lu in_bytes=%llu out_bytes=%llu\n",role,st.plain_in,st.plain_out,st.dtls_write_calls,st.dtls_read_records,st.plain_in_bytes,st.plain_out_bytes);
    return 0;
}

static int run_client(int listen_port,const char* transport_ip,int transport_port,const char* ca,const char* host){
    WOLFSSL_CTX* ctx=wolfSSL_CTX_new(wolfDTLSv1_3_client_method());if(!ctx){fprintf(stderr,"ctx client failed\n");return 2;}
    wolfSSL_CTX_set_verify(ctx,WOLFSSL_VERIFY_PEER,NULL);
    if(wolfSSL_CTX_load_verify_locations(ctx,ca,NULL)!=WOLFSSL_SUCCESS){fprintf(stderr,"CA load failed\n");return 2;}
    int t=socket(AF_INET,SOCK_DGRAM,0);if(t<0)die("transport socket");timeout_fd(t);struct sockaddr_in ta=addr4(transport_ip,transport_port);if(connect(t,(struct sockaddr*)&ta,sizeof(ta))<0)die("transport connect");
    WOLFSSL* ssl=wolfSSL_new(ctx);if(!ssl)return 2;if(wolfSSL_set_fd(ssl,t)!=WOLFSSL_SUCCESS)return 2;if(wolfSSL_check_domain_name(ssl,host)!=WOLFSSL_SUCCESS)return 2;
    int r=wolfSSL_connect(ssl);if(r!=WOLFSSL_SUCCESS){ssl_log("client handshake",ssl,r);return 3;}
    int p=socket(AF_INET,SOCK_DGRAM,0);if(p<0)die("plain socket");struct sockaddr_in pa=addr4("127.0.0.1",listen_port);if(bind(p,(struct sockaddr*)&pa,sizeof(pa))<0)die("plain bind");
    printf("READY role=client version=%s cipher=%s listen=%d\n",wolfSSL_get_version(ssl),wolfSSL_get_cipher(ssl),listen_port);fflush(stdout);
    int rc=relay_loop("client",ssl,t,p,1);wolfSSL_free(ssl);wolfSSL_CTX_free(ctx);close(t);close(p);return rc;
}
static int run_server(int listen_port,const char* target_ip,int target_port,const char* cert,const char* key){
    WOLFSSL_CTX* ctx=wolfSSL_CTX_new(wolfDTLSv1_3_server_method());if(!ctx)return 2;
    if (wolfSSL_CTX_use_certificate_chain_file(ctx,cert)!=WOLFSSL_SUCCESS) return 2;
    if (wolfSSL_CTX_use_PrivateKey_file(ctx,key,WOLFSSL_FILETYPE_PEM)!=WOLFSSL_SUCCESS) return 2;
    int t=socket(AF_INET,SOCK_DGRAM,0);if(t<0)die("transport socket");timeout_fd(t);struct sockaddr_in la=addr4("127.0.0.1",listen_port);if(bind(t,(struct sockaddr*)&la,sizeof(la))<0)die("transport bind");
    unsigned char peek[2048];struct sockaddr_in peer;socklen_t plen=sizeof(peer);int n=(int)recvfrom(t,peek,sizeof(peek),MSG_PEEK,(struct sockaddr*)&peer,&plen);if(n<=0)die("peek");
    WOLFSSL* ssl=wolfSSL_new(ctx);if(!ssl)return 2;if(wolfSSL_set_fd(ssl,t)!=WOLFSSL_SUCCESS)return 2;if(wolfSSL_dtls_set_peer(ssl,&peer,plen)!=WOLFSSL_SUCCESS)return 2;if(wolfSSL_send_hrr_cookie(ssl,NULL,0)!=WOLFSSL_SUCCESS)return 2;
    int r=wolfSSL_accept(ssl);if(r!=WOLFSSL_SUCCESS){ssl_log("server handshake",ssl,r);return 3;}
    int p=socket(AF_INET,SOCK_DGRAM,0);if(p<0)die("plain socket");struct sockaddr_in ta=addr4(target_ip,target_port);if(connect(p,(struct sockaddr*)&ta,sizeof(ta))<0)die("plain target connect");
    printf("READY role=server version=%s cipher=%s target=%s:%d\n",wolfSSL_get_version(ssl),wolfSSL_get_cipher(ssl),target_ip,target_port);fflush(stdout);
    int rc=relay_loop("server",ssl,t,p,0);wolfSSL_free(ssl);wolfSSL_CTX_free(ctx);close(t);close(p);return rc;
}
int main(int argc,char**argv){
    signal(SIGTERM,on_signal);signal(SIGINT,on_signal);if(wolfSSL_Init()!=WOLFSSL_SUCCESS)return 2;
    int rc=2;
    if(argc==7&&strcmp(argv[1],"client")==0)rc=run_client(atoi(argv[2]),argv[3],atoi(argv[4]),argv[5],argv[6]);
    else if(argc==7&&strcmp(argv[1],"server")==0)rc=run_server(atoi(argv[2]),argv[3],atoi(argv[4]),argv[5],argv[6]);
    else fprintf(stderr,"usage: %s client PLAIN_LISTEN TRANSPORT_IP TRANSPORT_PORT CA HOST | server TRANSPORT_LISTEN TARGET_IP TARGET_PORT CERT KEY\n",argv[0]);
    wolfSSL_Cleanup();return rc;
}
