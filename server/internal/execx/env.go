package execx

import "os"

func execEnvList() []string { return os.Environ() }
