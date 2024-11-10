// Copyright 2023-2024 Deepgram SDK contributors. All Rights Reserved.
// Use of this source code is governed by a MIT license that can be found in the LICENSE file.
// SPDX-License-Identifier: MIT

package main

// streaming
import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	api "github.com/deepgram/deepgram-go-sdk/pkg/api/listen/v1/websocket/interfaces"
	interfaces "github.com/deepgram/deepgram-go-sdk/pkg/client/interfaces"
	client "github.com/deepgram/deepgram-go-sdk/pkg/client/listen"

	microphone "DeepgramOnlineTranslator/microphone"
)

// Implement your own callback
type MyCallback struct {
	startAt         time.Time
	unsavedSentence *strings.Builder
	transcriptFile  *os.File
}

func (c MyCallback) Message(mr *api.MessageResponse) error {
	// handle the message
	sentence := strings.TrimSpace(mr.Channel.Alternatives[0].Transcript)

	if len(mr.Channel.Alternatives) == 0 || len(sentence) == 0 {
		return nil
	}

	if mr.IsFinal {

		duration := time.Duration(mr.Start * float64(time.Second))
		sentenceTime := fmt.Sprintf("[%s]", c.startAt.Add(duration).Format("15:04:05"))

		c.transcriptFile.WriteString(sentenceTime)
		c.transcriptFile.WriteString(" ")
		c.transcriptFile.WriteString(sentence)
		c.transcriptFile.WriteString("\n")
		c.unsavedSentence.Reset()

		fmt.Printf("\r%v %s", sentenceTime, sentence)
		fmt.Println()

		if mr.SpeechFinal {
			c.transcriptFile.WriteString("\n")
			fmt.Println()
		}
	} else {
		fmt.Printf("\r[........] %s", sentence)

		c.unsavedSentence.WriteString(sentence)
		c.unsavedSentence.WriteString(" ")
	}

	return nil
}

func (c MyCallback) Open(ocr *api.OpenResponse) error {
	// handle the open
	//fmt.Printf("\n[Open] Received\n")
	return nil
}

func (c MyCallback) Metadata(md *api.MetadataResponse) error {
	// handle the metadata
	fmt.Printf("\n[Metadata] Received\n")
	fmt.Printf("Metadata.RequestID: %s\n", strings.TrimSpace(md.RequestID))
	fmt.Printf("Metadata.Channels: %d\n", md.Channels)
	fmt.Printf("Metadata.Created: %s\n\n", strings.TrimSpace(md.Created))
	return nil
}

func (c MyCallback) SpeechStarted(ssr *api.SpeechStartedResponse) error {
	//fmt.Printf("\n[SpeechStarted] Received\n")
	return nil
}

func (c MyCallback) UtteranceEnd(ur *api.UtteranceEndResponse) error {

	return nil
}

func (c MyCallback) Close(ocr *api.CloseResponse) error {
	// handle the close
	fmt.Printf("\n[Close] Received\n")

	if c.unsavedSentence.Len() > 0 {
		fmt.Printf("\n[Close] Saving unsaved: %s\n", c.unsavedSentence.String())
		c.transcriptFile.WriteString(c.unsavedSentence.String())
		c.transcriptFile.WriteString("\n")
		c.unsavedSentence.Reset()
	}

	c.transcriptFile.Close()

	return nil
}

func (c MyCallback) Error(er *api.ErrorResponse) error {
	// handle the error
	fmt.Printf("\n[Error] Received\n")
	fmt.Printf("Error.Type: %s\n", er.Type)
	fmt.Printf("Error.ErrCode: %s\n", er.ErrCode)
	fmt.Printf("Error.Description: %s\n\n", er.Description)
	return nil
}

func (c MyCallback) UnhandledEvent(byData []byte) error {
	// handle the unhandled event
	fmt.Printf("\n[UnhandledEvent] Received\n")
	fmt.Printf("UnhandledEvent: %s\n\n", string(byData))
	return nil
}

func main() {
	// init library
	microphone.Initialize()

	// print instructions
	fmt.Print("\n\nPress ENTER to exit!\n\n")

	/*
		DG Streaming API
	*/
	// init library
	client.Init(client.InitLib{
		LogLevel: client.LogLevelDefault, // LogLevelDefault, LogLevelFull, LogLevelDebug, LogLevelTrace
	})

	// Go context
	ctx := context.Background()

	// client options
	cOptions := &interfaces.ClientOptions{
		EnableKeepAlive: true,
	}

	// set the Transcription options
	tOptions := &interfaces.LiveTranscriptionOptions{
		Model:       "nova-2",
		Language:    "en-US",
		Punctuate:   true,
		Encoding:    "linear16",
		Channels:    1,
		SampleRate:  16000,
		SmartFormat: true,
		VadEvents:   true,
		// To get UtteranceEnd, the following must be set:
		InterimResults: true,
		UtteranceEndMs: "1000",
		// End of UtteranceEnd settings
	}

	// example on how to send a custom parameter
	// params := make(map[string][]string, 0)
	// params["dictation"] = []string{"true"}
	// ctx = interfaces.WithCustomParameters(ctx, params)

	now := time.Now()
	transcriptFileName := "transcript_" + now.Format("20060102_150405.000.txt")
	fmt.Printf("Transcript to : %s\n", transcriptFileName)
	f, err := os.Create(transcriptFileName)
	if err != nil {
		panic(err)
	}

	// implement your own callback
	callback := MyCallback{
		unsavedSentence: &strings.Builder{},
		transcriptFile:  f,
		startAt:         now,
	}

	// create a Deepgram client
	dgClient, err := client.NewWSUsingCallback(ctx, "", cOptions, tOptions, callback)
	if err != nil {
		fmt.Println("ERROR creating LiveTranscription connection:", err)
		return
	}

	// connect the websocket to Deepgram
	bConnected := dgClient.Connect()
	if !bConnected {
		fmt.Println("Client.Connect failed")
		os.Exit(1)
	}

	/*
		Microphone package
	*/
	// mic stuf
	mic, err := microphone.New(microphone.AudioConfig{
		DeviceNameRx:  `CABLE\s*Output`,
		InputChannels: 1,
		SamplingRate:  16000,
	})
	if err != nil {
		fmt.Printf("Initialize failed. Err: %v\n", err)
		os.Exit(1)
	}

	// start the mic
	err = mic.Start()
	if err != nil {
		fmt.Printf("mic.Start failed. Err: %v\n", err)
		os.Exit(1)
	}

	go func() {
		// feed the microphone stream to the Deepgram client (this is a blocking call)
		mic.Stream(dgClient)
	}()

	// wait for user input to exit
	input := bufio.NewScanner(os.Stdin)
	input.Scan()

	// close mic stream
	err = mic.Stop()
	if err != nil {
		fmt.Printf("mic.Stop failed. Err: %v\n", err)
		os.Exit(1)
	}

	// teardown library
	microphone.Teardown()

	// close DG client
	dgClient.Stop()

	fmt.Printf("\n\nProgram exiting...\n")
}
