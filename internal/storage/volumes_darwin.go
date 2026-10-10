//go:build darwin

package storage

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os/exec"
)

func parseDiskInfo(b []byte) (volumeInfo, error) {
	if len(b) > 1<<20 {
		return volumeInfo{}, errors.New("volume metadata exceeds limit")
	}
	d := xml.NewDecoder(bytes.NewReader(b))
	info := volumeInfo{}
	for {
		token, err := d.Token()
		if err != nil {
			return info, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "dict" {
			break
		}
	}
	for {
		token, err := d.Token()
		if err != nil {
			return info, err
		}
		if end, ok := token.(xml.EndElement); ok && end.Name.Local == "dict" {
			return info, nil
		}
		key, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		if key.Name.Local != "key" {
			return info, errors.New("invalid volume metadata dictionary")
		}
		var name string
		if err = d.DecodeElement(&name, &key); err != nil {
			return info, err
		}
		var value xml.StartElement
		for {
			token, err = d.Token()
			if err != nil {
				return info, err
			}
			if start, ok := token.(xml.StartElement); ok {
				value = start
				break
			}
			if _, ended := token.(xml.EndElement); ended {
				return info, io.ErrUnexpectedEOF
			}
		}
		if name == "MountPoint" || name == "VolumeUUID" {
			if value.Name.Local != "string" {
				return info, errors.New("invalid volume identity metadata")
			}
			var text string
			if err = d.DecodeElement(&text, &value); err != nil {
				return info, err
			}
			if name == "MountPoint" {
				info.MountPoint = text
			} else {
				info.UUID = text
			}
		} else if err = d.Skip(); err != nil {
			return info, err
		}
	}
}

func lookupVolume(ctx context.Context, point string) (volumeInfo, error) {
	b, err := exec.CommandContext(ctx, "/usr/sbin/diskutil", "info", "-plist", point).Output()
	if err != nil {
		return volumeInfo{}, errors.New("macOS volume metadata is unavailable")
	}
	return parseDiskInfo(b)
}
