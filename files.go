package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"time"

	tsraw "github.com/multiplay/go-ts3"
)

// iconDir is the channel 0 directory where the server keeps icons.
// TeamSpeak's own snapshot only references icons by id.
const iconDir = "/icons"

const (
	errEmptyResult = 1281
	errFlooding    = 524
)

func backupIcons(c *tsraw.Client, ftAddr string) (map[string][]byte, error) {
	var entries []struct {
		Name string `ms:"name"`
	}
	err := execRetry(c, tsraw.NewCmd("ftgetfilelist").WithArgs(
		tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("path", iconDir),
	).WithResponse(&entries))
	var qerr *tsraw.Error
	if errors.As(err, &qerr) && qerr.ID == errEmptyResult {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list icons: %w", err)
	}

	files := map[string][]byte{}
	for _, e := range entries {
		data, err := download(c, ftAddr, "/"+e.Name)
		if err != nil {
			return nil, fmt.Errorf("download %s: %w", e.Name, err)
		}
		files[path.Join(iconDir, e.Name)] = data
	}
	return files, nil
}

func restoreIcons(c *tsraw.Client, ftAddr string, files map[string][]byte) error {
	for p, data := range files {
		dir, name := path.Split(p)
		if dir != iconDir+"/" {
			return fmt.Errorf("unsupported file in snapshot: %s", p)
		}
		// Icons are uploaded by bare name; the server files them under iconDir.
		if err := upload(c, ftAddr, "/"+name, data); err != nil {
			return fmt.Errorf("upload %s: %w", name, err)
		}
	}
	return nil
}

// ftInitResponse is the answer to ftinitdownload and ftinitupload, which
// report failure in its status field instead of as a query error.
type ftInitResponse struct {
	Key    string `ms:"ftkey"`
	Size   int64  `ms:"size"`
	Status int    `ms:"status"`
	Msg    string `ms:"msg"`
}

func ftInit(c *tsraw.Client, cmd string, args ...tsraw.CmdArg) (ftInitResponse, error) {
	var resp ftInitResponse
	err := execRetry(c, tsraw.NewCmd(cmd).WithArgs(args...).WithResponse(&resp))
	if err == nil && resp.Status != 0 {
		err = fmt.Errorf("%s (%d)", resp.Msg, resp.Status)
	}
	return resp, err
}

func download(c *tsraw.Client, ftAddr, name string) ([]byte, error) {
	resp, err := ftInit(c, "ftinitdownload",
		tsraw.NewArg("clientftfid", 1), tsraw.NewArg("name", name),
		tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("seekpos", 0))
	if err != nil {
		return nil, err
	}
	conn, err := ftDial(ftAddr, resp.Key)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	data := make([]byte, resp.Size)
	_, err = io.ReadFull(conn, data)
	return data, err
}

func upload(c *tsraw.Client, ftAddr, name string, data []byte) error {
	resp, err := ftInit(c, "ftinitupload",
		tsraw.NewArg("clientftfid", 1), tsraw.NewArg("name", name),
		tsraw.NewArg("cid", 0), tsraw.NewArg("cpw", ""), tsraw.NewArg("size", len(data)),
		tsraw.NewArg("overwrite", 1), tsraw.NewArg("resume", 0))
	if err != nil {
		return err
	}
	conn, err := ftDial(ftAddr, resp.Key)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.Write(data); err != nil {
		return err
	}
	// The server closes the connection once it has stored all bytes.
	_, err = io.Copy(io.Discard, conn)
	return err
}

// ftDial opens a file transfer connection and authenticates it with key.
func ftDial(addr, key string) (net.Conn, error) {
	conn, err := net.DialTimeout("tcp", addr, 10*time.Second)
	if err != nil {
		return nil, err
	}
	conn.SetDeadline(time.Now().Add(time.Minute))
	if _, err := io.WriteString(conn, key); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// execRetry runs cmd, waiting out the server's flood protection. A command
// rejected for flooding (524) was not run, so retrying cannot apply it twice.
// Quick repeated 524s get the IP banned (serverinstance_serverquery_ban_time),
// so each retry waits a full default flood window
// (serverinstance_serverquery_flood_time=3).
func execRetry(c *tsraw.Client, cmd *tsraw.Cmd) error {
	for i := 0; ; i++ {
		_, err := c.ExecCmd(cmd)
		var qerr *tsraw.Error
		if !errors.As(err, &qerr) || qerr.ID != errFlooding || i == 10 {
			return err
		}
		time.Sleep(3 * time.Second)
	}
}
