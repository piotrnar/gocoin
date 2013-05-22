#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <sys/time.h>


#ifdef WINDOWS
	#include <windows.h>
#else
	#include <sys/types.h>
	#include <sys/socket.h>
	#include <netinet/in.h>
	typedef int SOCKET;
	#define closesocket close
#endif

#include <openssl/ec.h>
#include <openssl/ecdsa.h>
#include <openssl/obj_mac.h>


static int inport = 16667;
static char bind_to[256] = "127.0.0.1";


static int verify(unsigned char *pkey, unsigned int pkl,
	unsigned char *sign, unsigned int sil, unsigned char *hasz) {
	EC_KEY* ecpkey = EC_KEY_new_by_curve_name(NID_secp256k1);
	if (!ecpkey) {
		printf("EC_KEY_new_by_curve_name error!\n");
		return 0;
	}
	if (!o2i_ECPublicKey(&ecpkey, (const unsigned char **)&pkey, pkl)) {
		printf("o2i_ECPublicKey fail!\n");
		while (pkl>0) {
			printf("%02x", *pkey);
			pkey++;
			pkl--;
		}
		printf("\n");
		return 0;
	}
	int res = ECDSA_verify(0, hasz, 32, sign, sil, ecpkey);
	EC_KEY_free(ecpkey);
	return res==1;
}

int readall(SOCKET sock, unsigned char *p, int l) {
	int i, lensofar = 0;
	while (lensofar<l) {
		i = recv(sock, p+lensofar, l-lensofar, 0);
		if (i<=0) {
			return -1;
		}
		lensofar+= i;
	}
	return 0;
}


#ifdef WINDOWS
DWORD WINAPI one_server(LPVOID par) {
#else
void *one_server(void *par) {
#endif
	unsigned char buf[256];
	SOCKET sock;

	memcpy(&sock, par, sizeof sock);
	free(par);

	if (readall(sock, buf, 256)) {
		printf("Socket read error\n");
		goto err;
	}

	if (buf[0]==1) { // ECDSA verify
		buf[0] = verify(buf+16, buf[1], buf+128, buf[2], buf+224)==1;
		if (send(sock, buf, 1, 0)!=1) {
			printf("Send error\n");
		}
	}

err:
	//printf("Closing socket %d\n", sock);
	closesocket(sock);
#ifdef WINDOWS
	return 0;
#else
	return NULL;
#endif
}



int main( int argc, char **argv )
{
	unsigned int totcnt = 0;
	time_t prv, now;
#ifdef WINDOWS
	WSADATA wsdata;
	WSAStartup(MAKEWORD(2, 2), &wsdata);
#endif

	struct sockaddr_in addr;
	memset(&addr, 0, sizeof(addr));
	addr.sin_family = AF_INET;
	addr.sin_port = htons(inport);
	addr.sin_addr.s_addr = inet_addr(bind_to);

	SOCKET sock = socket( AF_INET, SOCK_STREAM, 0 );
	if (sock==-1) {
		fprintf( stderr, "Cannot create socket\n" );
		return 1;
	}
	int yes = 1;
	if ( setsockopt(sock, SOL_SOCKET, SO_REUSEADDR, (void*)&yes, sizeof(int)) == -1 ) {
		fprintf(stderr, "SO_REUSEADDR failed\n");
		return 1;
	}

	if (bind(sock, (struct sockaddr*)&addr, sizeof(addr))==-1) {
		fprintf(stderr, "Cannot bind do specified port\n");
		return 1;
	}

	listen(sock, 5);
	printf("TCP server listening at %s:%d\n", bind_to, inport);
	prv = time(NULL);
	while (1) {
		int len = sizeof addr;
		memset(&addr, 0, len);
		SOCKET clnt = accept(sock, (struct sockaddr*)&addr, &len);
		if (clnt==-1) {
			fprintf( stderr, "Cannot accept connection\n" );
			continue;
		}
		//printf("Socket %d connected from %s\n", clnt, inet_ntoa(addr.sin_addr));
		void *tmp = malloc(sizeof clnt);
		memcpy(tmp, &clnt, sizeof clnt);
#ifdef WINDOWS
		if (!CreateThread(NULL, 0, one_server, tmp, 0, NULL)) {
#else
		pthread_t tid;
		if (pthread_create(&tid, NULL, one_server, tmp)) {
#endif
			fprintf( stderr, "Cannot create thread\n" );
			free(tmp);
			closesocket(clnt);
		}
#ifndef WINDOWS
		pthread_detach(tid);
#endif
		totcnt++;
		now = time(NULL);
		if (now!=prv) {
			if (now-prv == 1) {
				printf("%u: %u op / sec\n", (unsigned)now, totcnt/(unsigned int)(now-prv));
			} else {
				printf("%u: %u op\n", (unsigned)now, totcnt);
			}
			prv = now;
			totcnt = 0;
		}
	}

	return 0;
}
