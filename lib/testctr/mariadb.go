//
// See the file COPYRIGHT for copyright information.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package testctr

import (
	"context"
	"errors"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"log/slog"
)

const (
	MariaDBVersion     = "10.5.27"
	MariaDBDockerImage = "mariadb:" + MariaDBVersion
)

// MariaDBContainer creates and runs a MariaDB TestContainer.
//
// If there is an error on startup, this function will terminate the TestContainer before returning.
// After calling this function, the caller must be sure to defer cleanup, e.g. by `t.Cleanup(cleanup)`.
func MariaDBContainer(ctx context.Context, database, username, password string) (
	ctr testcontainers.Container,
	cleanup func(),
	port int32,
	err error,
) {
	var errs []error
	ctr, err = testcontainers.GenericContainer(
		ctx,
		testcontainers.GenericContainerRequest{
			ContainerRequest: testcontainers.ContainerRequest{
				Image:        MariaDBDockerImage,
				ExposedPorts: []string{"3306/tcp"},
				WaitingFor:   wait.ForLog("port: 3306  mariadb.org binary distribution"),
				Env: map[string]string{
					"MARIADB_RANDOM_ROOT_PASSWORD": "true",
					"MARIADB_DATABASE":             database,
					"MARIADB_USER":                 username,
					"MARIADB_PASSWORD":             password,
				},
			},
			Started: true,
		},
	)
	cleanup = func() {
		err := ctr.Terminate(ctx)
		if err != nil {
			slog.Error("Failed to terminate container", "error", err)
		}
	}
	errs = append(errs, err)
	natPort, err := ctr.MappedPort(ctx, "3306/tcp")
	errs = append(errs, err)
	dbHostPort := int32(natPort.Int())

	err = errors.Join(errs...)
	if err != nil {
		cleanup()
	}
	return ctr, cleanup, dbHostPort, errors.Join(errs...)
}
