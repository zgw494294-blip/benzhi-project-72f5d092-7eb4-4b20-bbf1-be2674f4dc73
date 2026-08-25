package persistence

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
)

const maxFrameSize = 32 << 20

func makeFrame(sequence int64, previous string, payload logPayload) (logFrame, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return logFrame{}, err
	}
	checksum := sha256.Sum256(encoded)
	frame := logFrame{SchemaVersion: schemaVersion, Sequence: sequence, PreviousDigest: previous, Payload: encoded, Checksum: hex.EncodeToString(checksum[:])}
	frame.Digest = frameDigest(frame.Sequence, frame.PreviousDigest, frame.Checksum, frame.Payload)
	return frame, nil
}

func frameDigest(sequence int64, previous, checksum string, payload []byte) string {
	hash := sha256.New()
	var sequenceBytes [8]byte
	binary.BigEndian.PutUint64(sequenceBytes[:], uint64(sequence))
	hash.Write(sequenceBytes[:])
	hash.Write([]byte(previous))
	hash.Write([]byte(checksum))
	hash.Write(payload)
	return hex.EncodeToString(hash.Sum(nil))
}

func appendFrame(file *os.File, frame logFrame) error {
	data, err := json.Marshal(frame)
	if err != nil {
		return err
	}
	if len(data) > maxFrameSize {
		return fmt.Errorf("日志帧过大")
	}
	var prefix [4]byte
	binary.BigEndian.PutUint32(prefix[:], uint32(len(data)))
	if _, err = file.Write(prefix[:]); err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		return err
	}
	return file.Sync()
}

func readFrames(path string) ([]logFrame, int64, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()
	reader := bufio.NewReader(file)
	frames := make([]logFrame, 0)
	var validBytes int64
	for {
		var prefix [4]byte
		_, err = io.ReadFull(reader, prefix[:])
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return nil, validBytes, err
		}
		size := binary.BigEndian.Uint32(prefix[:])
		if size == 0 || size > maxFrameSize {
			return nil, validBytes, fmt.Errorf("日志帧长度无效: %d", size)
		}
		data := make([]byte, size)
		_, err = io.ReadFull(reader, data)
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return nil, validBytes, err
		}
		var frame logFrame
		if err = json.Unmarshal(data, &frame); err != nil {
			return nil, validBytes, fmt.Errorf("日志中段 JSON 损坏: %w", err)
		}
		if err = validateFrame(frame, int64(len(frames)+1), digestBefore(frames)); err != nil {
			return nil, validBytes, err
		}
		frames = append(frames, frame)
		validBytes += int64(4 + size)
	}
	return frames, validBytes, nil
}

func validateFrame(frame logFrame, sequence int64, previous string) error {
	if frame.SchemaVersion != schemaVersion {
		return fmt.Errorf("不支持的日志 schemaVersion: %d", frame.SchemaVersion)
	}
	if frame.Sequence != sequence {
		return fmt.Errorf("日志序号不连续: 得到 %d，期望 %d", frame.Sequence, sequence)
	}
	if frame.PreviousDigest != previous {
		return fmt.Errorf("日志前序摘要不匹配，序号 %d", frame.Sequence)
	}
	checksum := sha256.Sum256(frame.Payload)
	if frame.Checksum != hex.EncodeToString(checksum[:]) {
		return fmt.Errorf("日志校验和不匹配，序号 %d", frame.Sequence)
	}
	if frame.Digest != frameDigest(frame.Sequence, frame.PreviousDigest, frame.Checksum, frame.Payload) {
		return fmt.Errorf("日志摘要不匹配，序号 %d", frame.Sequence)
	}
	return nil
}

func digestBefore(frames []logFrame) string {
	if len(frames) == 0 {
		return ""
	}
	return frames[len(frames)-1].Digest
}
