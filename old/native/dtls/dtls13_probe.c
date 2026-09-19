#include <wolfssl/options.h>
#include <wolfssl/ssl.h>
#include <arpa/inet.h>
#include <errno.h>
#include <netinet/in.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/socket.h>
#include <sys/time.h>
#include <unistd.h>

static void die(const char* what) { perror(what); exit(2); }
static void ssl_fail(const char* what, WOLFSSL* ssl, int ret) {
    int err = wolfSSL_get_error(ssl, ret);
    char buf[160];
    wolfSSL_ERR_error_string((unsigned long)err, buf);
    fprintf(stderr, "%s ret=%d err=%d msg=%s\n", what, ret, err, buf);
    exit(3);
}
static void timeout_fd(int fd) {
    struct timeval tv = {5,0};
    if (setsockopt(fd,SOL_SOCKET,SO_RCVTIMEO,&tv,sizeof(tv)) != 0) die("SO_RCVTIMEO");
    if (setsockopt(fd,SOL_SOCKET,SO_SNDTIMEO,&tv,sizeof(tv)) != 0) die("SO_SNDTIMEO");
}
static int server(int port, const char* cert, const char* key) {
    WOLFSSL_CTX* ctx = wolfSSL_CTX_new(wolfDTLSv1_3_server_method());
    if (!ctx) { fprintf(stderr,"server ctx failed\n"); return 2; }
    if (wolfSSL_CTX_use_certificate_chain_file(ctx, cert) != WOLFSSL_SUCCESS) {
        fprintf(stderr,"server cert load failed\n"); return 2;
    }
    if (wolfSSL_CTX_use_PrivateKey_file(ctx, key, WOLFSSL_FILETYPE_PEM) != WOLFSSL_SUCCESS) {
        fprintf(stderr,"server key load failed\n"); return 2;
    }
    int fd=socket(AF_INET,SOCK_DGRAM,0); if(fd<0) die("socket"); timeout_fd(fd);
    struct sockaddr_in sa={0}; sa.sin_family=AF_INET; sa.sin_addr.s_addr=htonl(INADDR_LOOPBACK); sa.sin_port=htons((unsigned short)port);
    if(bind(fd,(struct sockaddr*)&sa,sizeof(sa))!=0) die("bind");
    unsigned char peek[2048]; struct sockaddr_in peer={0}; socklen_t plen=sizeof(peer);
    int n=(int)recvfrom(fd,peek,sizeof(peek),MSG_PEEK,(struct sockaddr*)&peer,&plen);
    if(n<=0) die("server recvfrom peek");
    WOLFSSL* ssl=wolfSSL_new(ctx); if(!ssl){fprintf(stderr,"server ssl failed\n");return 2;}
    if(wolfSSL_set_fd(ssl,fd)!=WOLFSSL_SUCCESS){fprintf(stderr,"server set fd failed\n");return 2;}
    if(wolfSSL_dtls_set_peer(ssl,&peer,plen)!=WOLFSSL_SUCCESS){fprintf(stderr,"server set peer failed\n");return 2;}
    if(wolfSSL_send_hrr_cookie(ssl,NULL,0)!=WOLFSSL_SUCCESS){fprintf(stderr,"server HRR cookie failed\n");return 2;}
    int ret=wolfSSL_accept(ssl); if(ret!=WOLFSSL_SUCCESS) ssl_fail("server accept failed",ssl,ret);
    printf("HANDSHAKE_OK role=server version=%s cipher=%s\n",wolfSSL_get_version(ssl),wolfSSL_get_cipher(ssl)); fflush(stdout);
    char buf[128]={0}; ret=wolfSSL_read(ssl,buf,sizeof(buf)-1); if(ret<=0) ssl_fail("server read failed",ssl,ret);
    if(ret!=9 || memcmp(buf,"wbd-probe",9)!=0){fprintf(stderr,"server app mismatch len=%d\n",ret);return 4;}
    ret=wolfSSL_write(ssl,"wbd-ack",7); if(ret!=7) ssl_fail("server write failed",ssl,ret);
    wolfSSL_shutdown(ssl); wolfSSL_free(ssl); close(fd); wolfSSL_CTX_free(ctx); return 0;
}
static int client(int port, const char* ca, const char* host) {
    WOLFSSL_CTX* ctx=wolfSSL_CTX_new(wolfDTLSv1_3_client_method());
    if(!ctx){fprintf(stderr,"client ctx failed\n");return 2;}
    wolfSSL_CTX_set_verify(ctx,WOLFSSL_VERIFY_PEER,NULL);
    if(wolfSSL_CTX_load_verify_locations(ctx,ca,NULL)!=WOLFSSL_SUCCESS){fprintf(stderr,"client CA load failed\n");return 2;}
    int fd=socket(AF_INET,SOCK_DGRAM,0); if(fd<0) die("socket"); timeout_fd(fd);
    struct sockaddr_in sa={0}; sa.sin_family=AF_INET; sa.sin_addr.s_addr=htonl(INADDR_LOOPBACK); sa.sin_port=htons((unsigned short)port);
    if(connect(fd,(struct sockaddr*)&sa,sizeof(sa))!=0) die("connect udp");
    WOLFSSL* ssl=wolfSSL_new(ctx); if(!ssl){fprintf(stderr,"client ssl failed\n");return 2;}
    if(wolfSSL_set_fd(ssl,fd)!=WOLFSSL_SUCCESS){fprintf(stderr,"client set fd failed\n");return 2;}
    if(wolfSSL_check_domain_name(ssl,host)!=WOLFSSL_SUCCESS){fprintf(stderr,"client set hostname failed\n");return 2;}
    int ret=wolfSSL_connect(ssl);
    if(ret!=WOLFSSL_SUCCESS){
        int err=wolfSSL_get_error(ssl,ret); char ebuf[160]; wolfSSL_ERR_error_string((unsigned long)err,ebuf);
        fprintf(stderr,"HANDSHAKE_FAIL role=client host=%s ret=%d err=%d msg=%s\n",host,ret,err,ebuf);
        wolfSSL_free(ssl);close(fd);wolfSSL_CTX_free(ctx);return 10;
    }
    printf("HANDSHAKE_OK role=client host=%s version=%s cipher=%s\n",host,wolfSSL_get_version(ssl),wolfSSL_get_cipher(ssl)); fflush(stdout);
    ret=wolfSSL_write(ssl,"wbd-probe",9); if(ret!=9) ssl_fail("client write failed",ssl,ret);
    char buf[128]={0}; ret=wolfSSL_read(ssl,buf,sizeof(buf)-1); if(ret<=0) ssl_fail("client read failed",ssl,ret);
    if(ret!=7 || memcmp(buf,"wbd-ack",7)!=0){fprintf(stderr,"client app mismatch len=%d\n",ret);return 4;}
    printf("APP_OK reply=wbd-ack\n"); fflush(stdout);
    wolfSSL_shutdown(ssl); wolfSSL_free(ssl);close(fd);wolfSSL_CTX_free(ctx);return 0;
}
int main(int argc,char**argv){
    if(wolfSSL_Init()!=WOLFSSL_SUCCESS){fprintf(stderr,"wolfSSL_Init failed\n");return 2;}
    if(argc<2){fprintf(stderr,"usage: %s server PORT CERT KEY | client PORT CA HOST\n",argv[0]);return 2;}
    int rc;
    if(strcmp(argv[1],"server")==0 && argc==5) rc=server(atoi(argv[2]),argv[3],argv[4]);
    else if(strcmp(argv[1],"client")==0 && argc==5) rc=client(atoi(argv[2]),argv[3],argv[4]);
    else {fprintf(stderr,"bad args\n");rc=2;}
    wolfSSL_Cleanup(); return rc;
}
