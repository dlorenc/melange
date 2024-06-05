// Copyright 2024 Chainguard, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package sign

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"

	"chainguard.dev/apko/pkg/apk/expandapk"
	"chainguard.dev/melange/pkg/build"
)

// APK() signs an APK file with the provided key. The existing APK file is
// replaced with the signed APK file.
func APK(ctx context.Context, apkPath string, keyPath string) error {
	f, err := os.Open(apkPath)
	if err != nil {
		return err
	}
	defer f.Close()

	split, err := expandapk.Split(f)
	if err != nil {
		return fmt.Errorf("splitting apk: %w", err)
	}

	cf, df := split[0], split[1]
	if len(split) == 3 {
		// signature section is present
		cf, df = split[1], split[2]
	}

	pc := &build.PackageBuild{
		Build: &build.Build{
			SigningKey:        keyPath,
			SigningPassphrase: "",
		},
	}

	cdata, err := io.ReadAll(cf)
	if err != nil {
		return err
	}

	// Reading and writing to the same file seems risky, so we create a temp file.
	tmpData, err := os.CreateTemp("", "melange-sign-data-section-tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmpData.Name())

	if _, err := io.Copy(tmpData, df); err != nil {
		return err
	}
	if _, err := tmpData.Seek(0, 0); err != nil {
		return err
	}

	// Pull the modtime out of the .PKGINFO
	r := bytes.NewReader(cdata)
	zr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	hdr, err := tr.Next()
	if err != nil {
		return err
	}
	if hdr.Name != ".PKGINFO" {
		return fmt.Errorf("unexpected file in control section: %s", hdr.Name)
	}

	sigData, err := build.EmitSignature(ctx, pc.Signer(), cdata, hdr.ModTime)
	if err != nil {
		return err
	}

	w, err := os.Create(apkPath)
	if err != nil {
		return err
	}

	// Replace the package file with the new one
	for _, fp := range []io.Reader{bytes.NewReader(sigData), bytes.NewReader(cdata), tmpData} {
		if _, err := io.Copy(w, fp); err != nil {
			return err
		}
	}

	return w.Close()
}
