package filetransfer

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/pion/webrtc/v3"
)

type FileMetadata struct {
	FileName string `json:"fileName"`
	FileSize string `json:"fileSize"`
	FileHash string `json:"fileHash"`
}

type FileTransfer struct {
	Metadata     FileMetadata
	DataChannel  *webrtc.DataChannel
	TransferChan chan []byte //temporary holds received chunks of data
	mu           sync.Mutex
	receivedData []byte
}


func NewFileTransfer(dataChannel *webrtc.DataChannel) *FileTransfer {
	return &FileTransfer{
		DataChannel: dataChannel,
		TransferChan: make(chan []byte, 1024),
	}
}

func (ft *FileTransfer) SendFile(filePath string) error {
	//Read file

	fileData, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	hash := sha256.Sum256(fileData)

	//prepare metadata
	metadata := FileMetadata{
		FileName: filepath.Base(filePath),           // Use filepath.Base
		FileSize: fmt.Sprintf("%d", len(fileData)), // Convert file size to string
		FileHash: fmt.Sprintf("%x", hash),          // Convert hash to hex string
	}

	//sending metadata
	metadataJSON,_ := json.Marshal(metadata)
	ft.DataChannel.Send(metadataJSON)

	//chunk and sendfile
	chunkSize := 16 * 1024 // 16kb of chunks
	for i := 0; i < len(fileData); i+= chunkSize {
		end := i + chunkSize
		if end > len(fileData) {
			end = len(fileData)
		}
		chunk := fileData[i:end]
		ft.DataChannel.Send(chunk)
	}

	return nil
}

func (ft *FileTransfer) ReceiveFile() error {
	var metadata FileMetadata
	var receivedChunks [][]byte

	//wait for complete transfer

	for {
		select {
		case chunk := <- ft.TransferChan:
			//first chunk would be metadata

			if len(metadata.FileName) == 0 {
				err := json.Unmarshal(chunk, &metadata)

				if err != nil {
					return fmt.Errorf("invalid metadata")
				}

				continue
			}

			//accumulation
			receivedChunks = append(receivedChunks, chunk);

			//check if all chunks are received
			if fileSize, err := strconv.ParseInt(metadata.FileSize, 10, 64); err != nil {
				// Handle error in parsing FileSize
				return fmt.Errorf("invalid file size in metadata: %w", err)
			} else if int64(len(bytes.Join(receivedChunks, []byte{}))) == fileSize {
				// Call saveReceivedFile if the sizes match
				return ft.saveReceivedFile(metadata, receivedChunks)
			}
		}
	}
}

var receiveDirectory string = "."

func SetReceiveDirectory(dir string) {
    receiveDirectory = dir
}

func (ft *FileTransfer) saveReceivedFile(metadata FileMetadata, chunks [][]byte) error {
    // Combine chunks
    fileData := bytes.Join(chunks, []byte{})

    // Verify hash
    hash := sha256.Sum256(fileData)
    if fmt.Sprintf("%x", hash) != metadata.FileHash {
        return fmt.Errorf("file hash mismatch")
    }

    // Construct full file path
    fullPath := filepath.Join(receiveDirectory, metadata.FileName)

    // Save the file
    return os.WriteFile(fullPath, fileData, 0644) 
}
//integrating signaling server
func (ft *FileTransfer) SetupDataChannelHandlers() {
    ft.DataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
        ft.TransferChan <- msg.Data
    })
}
