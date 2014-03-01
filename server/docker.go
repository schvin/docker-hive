/*
   Copyright Evan Hazlett

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

/*
   These are structs from the Docker package to prevent the dependency
   on the Docker library.  We just use the structs so there is no need
   to bring in the external dependencies (libdevmapper, btrfs, etc.) just
   to use those.

*/
package server

import (
	"sync"
	"time"
)

type (
	Port        string
	PortBinding struct {
		HostIp   string
		HostPort string
	}
	PortSet map[Port]struct{}

	PortMap map[Port][]PortBinding

	State struct {
		sync.RWMutex
		Running    bool
		Pid        int
		ExitCode   int
		StartedAt  time.Time
		FinishedAt time.Time
		Ghost      bool
	}

	Container struct {
		Id              string
		Args            []string
		Config          ContainerConfig
		Created         time.Time
		Driver          string
		HostConfig      HostConfig
		HostnamePath    string
		HostsPath       string
		Image           string
		NetworkSettings NetworkSettings
		Path            string
		ResolvConfPath  string
		State           State
		Volumes         map[string]string
	}

	ContainerConfig struct {
		AttachStderr    bool
		AttachStdin     bool
		AttachStdout    bool
		Cmd             []string
		CpuShares       int64
		Dns             string
		Domainname      string
		Env             []string
		ExposedPorts    map[Port]struct{}
		Hostname        string
		Image           string
		Memory          int64
		MemorySwap      int64
		NetworkDisabled bool
		OnBuild         []string
		OpenStdin       bool
		PortSpecs       []string
		StdinOnce       bool
		Tty             bool
		User            string
		Volumes         map[string]struct{}
		VolumesFrom     string
	}

	KeyValuePair struct {
		Key   string
		Value string
	}

	HostConfig struct {
		Binds           []string
		ContainerIDFile string
		LxcConf         []KeyValuePair
		Privileged      bool
		PortBindings    PortMap
		Links           []string
		PublishAllPorts bool
	}

	PortMapping map[string]string

	NetworkSettings struct {
		IPAddress   string
		IPPrefixLen int
		Gateway     string
		Bridge      string
		PortMapping map[string]PortMapping
		Ports       PortMap
	}
)
