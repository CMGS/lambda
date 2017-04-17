package rpc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	"gopkg.in/yaml.v2"

	log "github.com/Sirupsen/logrus"
	"gitlab.ricebook.net/platform/core/rpc/gen"

	"google.golang.org/grpc"
)

const (
	appTmpl = `appname: "lambda"
entrypoints:
  %s:
    cmd: "%s"
    working_dir: "%s"
`
)

// [exitcode] bytes
var EXIT_CODE = []byte{91, 101, 120, 105, 116, 99, 111, 100, 101, 93, 32}

func RunAndWait(
	server, pod, image, name, command, network, workingDir string,
	envs, volumes []string, cpu float64, mem int64, count, timeout int) (code int) {

	conn, err := grpc.Dial(server, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}
	defer conn.Close()

	c := pb.NewCoreRPCClient(conn)
	opts := generateOpts(pod, image, name, command,
		network, workingDir, envs, volumes, cpu, mem, count)

	resp, err := c.RunAndWait(context.Background(), opts)
	if err != nil {
		log.Fatalf("Run failed %v", err)
	}

	// log container ids and clean
	containerIDs := NewCIDs()
	time.AfterFunc(time.Duration(timeout)*time.Second, func() { Remove(server, containerIDs) })

	for {
		msg, err := resp.Recv()
		if err == io.EOF {
			break
		}

		if err != nil {
			log.Fatalf("Message invalid %v", err)
		}

		// log container id
		containerIDs.Add(msg.ContainerId)

		if bytes.HasPrefix(msg.Data, EXIT_CODE) {
			ret := string(bytes.TrimLeft(msg.Data, string(EXIT_CODE)))
			code, err = strconv.Atoi(ret)
			if err != nil {
				log.Fatalf("exit with unknown %s %s", ret, err)
			}
			continue
		}
		data := msg.Data
		id := msg.ContainerId[:7]
		fmt.Printf("%s %s\n", id, data)
	}
	return
}

func generateOpts(pod, image, name, command, network, workingDir string,
	envs, volumes []string, cpu float64, mem int64, count int) *pb.DeployOptions {
	for i, env := range envs {
		envs[i] = fmt.Sprintf("LAMBDA_%s", env)
	}

	opts := &pb.DeployOptions{
		Specs:      generateSpecs(name, command, workingDir, volumes),
		Appname:    "lambda",
		Image:      image,
		Podname:    pod,
		Entrypoint: name,
		CpuQuota:   cpu,
		Memory:     mem,
		Count:      int32(count),
		Networks:   map[string]string{network: ""},
		Env:        envs,
	}
	return opts
}

func generateSpecs(name, command, workingDir string, volumes []string) string {
	specs := fmt.Sprintf(appTmpl, name, command, workingDir)
	if len(volumes) > 0 {
		vol := map[string][]string{}
		vol["volumes"] = volumes
		out, err := yaml.Marshal(vol)
		if err != nil {
			log.Fatalf("Parse failed %v", err)
		}
		specs = fmt.Sprintf("%s%s", specs, out)
	}
	return specs
}
