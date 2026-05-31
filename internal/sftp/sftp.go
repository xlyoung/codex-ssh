package sftp

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

// ProgressFunc is called during file transfer to report progress.
type ProgressFunc func(transferred, total int64, filename string)

// SSHConfig holds SSH connection parameters.
type SSHConfig struct {
	Host       string
	Port       int
	User       string
	Password   string
	KeyFile    string
	knownHosts string
}

// connect establishes an SSH connection.
func connect(cfg SSHConfig) (*ssh.Client, error) {
	port := cfg.Port
	if port == 0 {
		port = 22
	}

	var authMethods []ssh.AuthMethod

	if cfg.Password != "" {
		authMethods = append(authMethods, ssh.Password(cfg.Password))
	}

	if cfg.KeyFile != "" {
		key, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read key file: %w", err)
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, fmt.Errorf("parse key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}

	if len(authMethods) == 0 {
		return nil, fmt.Errorf("no authentication method available (set password or key file)")
	}

	config := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), // TODO: use known_hosts
		Timeout:         30 * time.Second,
	}

	addr := fmt.Sprintf("%s:%d", cfg.Host, port)
	return ssh.Dial("tcp", addr, config)
}

// Put uploads a local file to a remote server.
func Put(cfg SSHConfig, localPath, remotePath string, progress ProgressFunc) error {
	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open local file %s: %w", localPath, err)
	}
	defer localFile.Close()

	stat, err := localFile.Stat()
	if err != nil {
		return fmt.Errorf("stat local file: %w", err)
	}

	sshClient, err := connect(cfg)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("SFTP client: %w", err)
	}
	defer sftpClient.Close()

	// Create remote directory if needed
	remoteDir := filepath.Dir(remotePath)
	if err := sftpClient.MkdirAll(remoteDir); err != nil {
		return fmt.Errorf("create remote dir: %w", err)
	}

	remoteFile, err := sftpClient.Create(remotePath)
	if err != nil {
		return fmt.Errorf("create remote file: %w", err)
	}
	defer remoteFile.Close()

	if progress != nil {
		return copyWithProgress(localFile, remoteFile, stat.Size(), filepath.Base(localPath), progress)
	}

	_, err = io.Copy(remoteFile, localFile)
	return err
}

// Get downloads a remote file to local.
func Get(cfg SSHConfig, remotePath, localPath string, progress ProgressFunc) error {
	sshClient, err := connect(cfg)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("SFTP client: %w", err)
	}
	defer sftpClient.Close()

	remoteFile, err := sftpClient.Open(remotePath)
	if err != nil {
		return fmt.Errorf("open remote file: %w", err)
	}
	defer remoteFile.Close()

	stat, err := remoteFile.Stat()
	if err != nil {
		return fmt.Errorf("stat remote file: %w", err)
	}

	// Create local directory if needed
	localDir := filepath.Dir(localPath)
	if err := os.MkdirAll(localDir, 0755); err != nil {
		return fmt.Errorf("create local dir: %w", err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("create local file: %w", err)
	}
	defer localFile.Close()

	if progress != nil {
		return copyWithProgress(remoteFile, localFile, stat.Size(), filepath.Base(remotePath), progress)
	}

	_, err = io.Copy(localFile, remoteFile)
	return err
}

// Sync synchronizes a local directory to a remote directory.
func Sync(cfg SSHConfig, localDir, remoteDir string, progress ProgressFunc) error {
	sshClient, err := connect(cfg)
	if err != nil {
		return err
	}
	defer sshClient.Close()

	sftpClient, err := sftp.NewClient(sshClient)
	if err != nil {
		return fmt.Errorf("SFTP client: %w", err)
	}
	defer sftpClient.Close()

	var uploaded, skipped, failed int
	start := time.Now()

	err = filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(localDir, path)
		if err != nil {
			return err
		}
		remotePath := filepath.Join(remoteDir, relPath)

		if info.IsDir() {
			if err := sftpClient.MkdirAll(remotePath); err != nil {
				fmt.Printf("  ⚠️  mkdir %s: %v\n", remotePath, err)
				failed++
			}
			return nil
		}

		// Check if remote file exists and is same size
		if remoteStat, err := sftpClient.Stat(remotePath); err == nil {
			if remoteStat.Size() == info.Size() {
				skipped++
				return nil
			}
		}

		// Upload file
		localFile, err := os.Open(path)
		if err != nil {
			fmt.Printf("  ❌ open %s: %v\n", path, err)
			failed++
			return nil
		}
		defer localFile.Close()

		remoteFile, err := sftpClient.Create(remotePath)
		if err != nil {
			fmt.Printf("  ❌ create %s: %v\n", remotePath, err)
			failed++
			return nil
		}
		defer remoteFile.Close()

		if _, err := io.Copy(remoteFile, localFile); err != nil {
			fmt.Printf("  ❌ copy %s: %v\n", path, err)
			failed++
			return nil
		}

		uploaded++
		if progress != nil {
			progress(int64(uploaded), 0, relPath)
		}
		return nil
	})

	elapsed := time.Since(start)
	fmt.Printf("\n📊 Sync complete: %d uploaded, %d skipped, %d failed (%.1fs)\n", uploaded, skipped, failed, elapsed.Seconds())
	return err
}

// copyWithProgress copies from src to dst while reporting progress.
func copyWithProgress(src io.Reader, dst io.Writer, total int64, filename string, progress ProgressFunc) error {
	buf := make([]byte, 32*1024)
	var transferred int64
	start := time.Now()

	for {
		n, err := src.Read(buf)
		if n > 0 {
			written, werr := dst.Write(buf[:n])
			if werr != nil {
				return werr
			}
			transferred += int64(written)
			progress(transferred, total, filename)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}

	elapsed := time.Since(start)
	if elapsed.Seconds() > 0 {
		speed := float64(transferred) / elapsed.Seconds() / 1024 / 1024
		fmt.Printf("\n  ✅ %s: %d bytes in %.1fs (%.1f MB/s)\n", filename, transferred, elapsed.Seconds(), speed)
	}
	return nil
}

// ParseHost parses a host specification (user@host:port).
func ParseHost(spec string) SSHConfig {
	cfg := SSHConfig{Port: 22}

	if idx := strings.Index(spec, "@"); idx >= 0 {
		cfg.User = spec[:idx]
		spec = spec[idx+1:]
	}

	if idx := strings.Index(spec, ":"); idx >= 0 {
		cfg.Host = spec[:idx]
		fmt.Sscanf(spec[idx+1:], "%d", &cfg.Port)
	} else {
		cfg.Host = spec
	}

	return cfg
}
