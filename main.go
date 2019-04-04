// DiskDuplicator project main.go
package main

import (
	"bytes"
	"flag"
	"io"
	"log"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
	"io/ioutil"
	"os/exec"
	"os/signal"
	"crypto/md5"
	"encoding/hex"

	"gopkg.in/cheggaaa/pb.v1"
)

const (
	DiskPath = "/dev/disk/by-id"
)

var (
	DiskImageSize   int64 = -1
	DiskImagePath         = "image.img"
	DiskImageMD5Sum       = ""
	CheckOnly				= false
)

func main() {
	imgPath := flag.String("image-path", "image.img", "Path to the disk image.")
	verifyOnly := flag.Bool("verify-only", false, "Make this true to only verify, not write")
	flag.Parse()

	CheckOnly = *verifyOnly

	DiskImagePath = *imgPath
	os.Remove("errors.log")
	fLog, err := os.Create("errors.log")
	if err != nil {
		log.Println(err)
		debug.PrintStack()
		os.Exit(1)
	}
	log.SetOutput(fLog)
	defer fLog.Close()

	log.Println("Started at ", time.Now())

	runtime.GOMAXPROCS(runtime.NumCPU() * 10)
	cmd := exec.Command("/bin/bash", "-c", "gsettings set org.gnome.desktop.media-handling automount false")
	err = cmd.Run()
	if err != nil {
		log.Fatal("Unable to prevent Nautilus from auto-mounting drives : ", err)
	}
	defer exec.Command("/bin/bash", "-c", "gsettings set org.gnome.desktop.media-handling automount true").Run()

	{
		fSize, err := os.Stat(DiskImagePath)
		if err != nil {
			log.Fatal("Unable to find disk image.")
		}
		DiskImageSize = fSize.Size()
	}

	{
		log.Println("Summing source image...")
		MD5SummerChan := make(chan int64, 1000)
		go func() {
			defer close(MD5SummerChan)
			DiskImageMD5Sum = getMD5String(DiskImagePath, MD5SummerChan)
		}()

		pbar := pb.New64(DiskImageSize)
		for p := range MD5SummerChan {
			pbar.Add64(p)
		}
		pbar.FinishPrint("Done summing source image. MD5Sum Hash : " + DiskImageMD5Sum)
	}

	counter := 0
	var pbPool *pb.Pool
	defer pbPool.Stop()

	completedFiles := getFiles(DiskPath)
	for _, j := range completedFiles {
		log.Println(j)
	}

	firstPBRun := true
	runners := make([]chan bool, 0)
	ticky := time.NewTicker(time.Second)

	c := make(chan os.Signal, 10)
	signal.Notify(c, os.Interrupt)
	go func() {
		for range c {
			os.Exit(0)
			log.Println("Cleaning up...")
			go ticky.Stop()
			for _, j := range runners {
				for range j {
				}
			}

			if pbPool != nil {
				pbPool.Stop()
			}
			time.Sleep(time.Second)
			os.Exit(0)
		}
	}()

	log.Println("Disk Duplicator is ready.")

	for range ticky.C {
		currentFiles := getFiles(DiskPath)

		for _, j := range currentFiles {
			if !fileExists(completedFiles, j) {
				completedFiles = append(completedFiles, j)

				counter++
				mult := int64(2)
				if CheckOnly {
					mult = 1
				}
				pbar := pb.New64(DiskImageSize*mult)
				pbar.SetUnits(pb.U_BYTES)
				pbar.Prefix(strconv.Itoa(counter))
				defer pbar.Finish()
				if firstPBRun {
					firstPBRun = false
					pbPool, _ = pb.StartPool(pbar)
				} else {
					pbPool.Add(pbar)
				}

				running := make(chan bool, 1)
				runners = append(runners, running)
				go duplicateToDisk(j, pbar, running)
			}
		}
	}
}

func getFiles(path string) (files []string) {
	files = make([]string, 0)

	currentGUIDs, _ := ioutil.ReadDir(path)

	for _, j := range currentGUIDs {
		if strings.Contains(j.Name(), "usb") && !strings.Contains(j.Name(), "part") {
			files = append(files, path+"/"+j.Name())
		}
	}

	return
}

func duplicateToDisk(disk string, pBar *pb.ProgressBar, running chan bool) {
	defer close(running)
	running <- true

	pbarChan := make(chan int64, 100)
	go copyFileContents(DiskImagePath, disk, pbarChan)

	for j := range pbarChan {
		pBar.Add64(j)
	}
}

func copyFileContents(src, dst string, procBytes chan int64) (err error) {
	defer close(procBytes)
	if !CheckOnly {
		{
			var (
				in, out *os.File
			)

			in, err = os.Open(src)
			if err != nil {
				return
			}
			defer in.Close()

			out, err = os.Create(dst)
			if err != nil {
				return
			}
			defer func() {
				cerr := out.Close()
				if err == nil {
					err = cerr
				}
			}()

			incomingBytes := make(chan []byte, 10)
			go startReadingBytes(in, incomingBytes)

			for p := range incomingBytes {
				bytesWritten, _ := io.Copy(out, bytes.NewBuffer(p))

				procBytes <- bytesWritten
			}

			if err != nil {
				return
			}
			err = out.Sync()
		}
	}

	{ // clear out the read cache so we get can accurate checksum of the written data
		cmd := exec.Command("/bin/bash", "-c", "echo 3 > /proc/sys/vm/drop_caches")
		err = cmd.Run()
		if err != nil {
			cmd.Run()
		}
	}

	thisHash := getMD5String(dst, procBytes)
	if !strings.Contains(thisHash, DiskImageMD5Sum) {
		log.Println("BAD", src, dst, "\t\t!!!!!!")
	} else {
		log.Println("GOOD : ", src, dst)
	}

	return
}

func getMD5String(file string, procBytes chan int64) (thisHash string) {
		h := md5.New()

		in, err := os.Open(file)
		if err != nil {
			return
		}
		defer in.Close()

		incomingBytes := make(chan []byte, 10)
		go startReadingBytes(in, incomingBytes)

		for p := range incomingBytes {
			bytesWritten, _ := io.Copy(h, bytes.NewBuffer(p))

			procBytes <- bytesWritten
		}

		if err != nil {
			return
		}

		thisHash = hex.EncodeToString(h.Sum(nil))
		log.Println(thisHash)

		return
}

func startReadingBytes(in io.Reader, output chan []byte) {
	defer close(output)

	var bytesRead int64 = 0

	for {
		buffy := make([]byte, 1024*64) // 64KB
		toWrite, err := io.ReadFull(in, buffy)
		output <- buffy[:toWrite]
		if err != nil {
			break
		}

		bytesRead += int64(toWrite)

		if bytesRead == DiskImageSize {
			return
		} else if bytesRead > DiskImageSize {
			log.Println("bytesread is bigger than diskimagesize :(")
		}
	}
}

func fileExists(main []string, sub string) (exists bool) {
	for _, j := range main {
		if j == sub {
			return true
		}
	}

	return
}
