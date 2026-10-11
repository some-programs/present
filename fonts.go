package main

import (
	"archive/tar"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path"
	"strings"
	"time"
)

// fontsDir holds the TrueType fonts that fontsTarHandler serves.
const fontsDir = "static/fonts/ttf"

// fontLicenses are the license files of the fonts in fontsDir.
var fontLicenses = []string{
	"static/fonts/OFL-Roboto.txt",
	"static/fonts/OFL-JetBrainsMono.txt",
}

// fontsTarHandler serves a tar archive of the TrueType fonts in fsys and
// their licenses, for installing the fonts that presentations use, such
// as for diagrams to render with them. The archive holds a single
// directory, fonts.
func fontsTarHandler(fsys fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("Content-Disposition", `attachment; filename="fonts.tar"`)
		if err := writeFontsTar(w, fsys); err != nil {
			log.Printf("fonts.tar: %v", err)
			// Abort the response so that the client does not take a
			// truncated archive for a complete one.
			panic(http.ErrAbortHandler)
		}
	})
}

// writeFontsTar writes the tar archive that fontsTarHandler serves to w.
func writeFontsTar(w io.Writer, fsys fs.FS) error {
	tw := tar.NewWriter(w)
	// The embedded files are read-only and carry no modification time,
	// so the archive sets the modes and time they are extracted with.
	// Fontconfig rescans a font directory only when its time changes.
	now := time.Now()
	if err := tw.WriteHeader(&tar.Header{
		Typeflag: tar.TypeDir,
		Name:     "fonts/",
		Mode:     0o755,
		ModTime:  now,
	}); err != nil {
		return err
	}
	addFile := func(name, file string) error {
		data, err := fs.ReadFile(fsys, file)
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(data)),
			ModTime:  now,
		}); err != nil {
			return err
		}
		_, err = tw.Write(data)
		return err
	}
	for _, file := range fontLicenses {
		if err := addFile("fonts/"+path.Base(file), file); err != nil {
			return err
		}
	}
	err := fs.WalkDir(fsys, fontsDir, func(file string, d fs.DirEntry, err error) error {
		if err != nil || file == fontsDir {
			return err
		}
		name := "fonts/" + strings.TrimPrefix(file, fontsDir+"/")
		if d.IsDir() {
			return tw.WriteHeader(&tar.Header{
				Typeflag: tar.TypeDir,
				Name:     name + "/",
				Mode:     0o755,
				ModTime:  now,
			})
		}
		return addFile(name, file)
	})
	if err != nil {
		return err
	}
	return tw.Close()
}
