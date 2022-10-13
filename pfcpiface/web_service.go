// SPDX-License-Identifier: Apache-2.0
// Copyright 2020 Intel Corporation

package pfcpiface

import (
	"encoding/json"
	"io"
	"math"
	"net/http"

	log "github.com/sirupsen/logrus"
)

// NetworkSlice ... Config received for slice rates and DNN.
type NetworkSlice struct {
	SliceName string      `json:"sliceName"`
	SliceQos  SliceQos    `json:"sliceQos"`
	UeResInfo []UeResInfo `json:"ueResourceInfo"`
}

// SliceQos ... Slice level QOS rates.
type SliceQos struct {
	UplinkMbr    uint64 `json:"uplinkMbr"`
	DownlinkMbr  uint64 `json:"downlinkMbr"`
	BitrateUnit  string `json:"bitrateUnit"`
	UlBurstBytes uint64 `json:"uplinkBurstSize"`
	DlBurstBytes uint64 `json:"downlinkBurstSize"`
}

// UeResInfo ... UE Pool and DNN info.
type UeResInfo struct {
	Dnn  string `json:"dnn"`
	Name string `json:"uePoolId"`
}

type ConfigHandler struct {
	upf *upf
}

func setupConfigHandler(mux *http.ServeMux, upf *upf) {
	cfgHandler := ConfigHandler{upf: upf}
	mux.Handle("/v1/config/network-slices", &cfgHandler)
}

func (c *ConfigHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Infoln("handle http request for /v1/config/network-slices")

	switch r.Method {
	case "PUT":
		fallthrough
	case "POST":
		body, err := io.ReadAll(r.Body)
		if err != nil {
			log.Errorln("http req read body failed.")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		log.Traceln(string(body))

		var nwSlice NetworkSlice

		err = json.Unmarshal(body, &nwSlice)
		if err != nil {
			log.Errorln("Json unmarshal failed for http request")
			sendHTTPResp(http.StatusBadRequest, w)
		}

		handleSliceConfig(&nwSlice, c.upf)
		sendHTTPResp(http.StatusCreated, w)
	default:
		log.Infoln(w, "Sorry, only PUT and POST methods are supported.")
		sendHTTPResp(http.StatusMethodNotAllowed, w)
	}
}

func sendHTTPResp(status int, w http.ResponseWriter) {
	w.WriteHeader(status)
	w.Header().Set("Content-Type", "application/json")

	resp := make(map[string]string)

	switch status {
	case http.StatusCreated:
		resp["message"] = "Status Created"
	default:
		resp["message"] = "Failed to add slice"
	}

	jsonResp, err := json.Marshal(resp)
	if err != nil {
		log.Errorln("Error happened in JSON marshal. Err: ", err)
	}

	_, err = w.Write(jsonResp)
	if err != nil {
		log.Errorln("http response write failed : ", err)
	}
}

// calculateBitRates : Default bit rate is Mbps.
func calculateBitRates(mbr uint64, rate string) uint64 {
	var val int64

	switch rate {
	case "bps":
		return mbr
	case "Kbps":
		val = int64(mbr) * KB
	case "Gbps":
		val = int64(mbr) * GB
	case "Mbps":
		fallthrough
	default:
		val = int64(mbr) * MB
	}

	if val > 0 {
		return uint64(val)
	} else {
		return uint64(math.MaxInt64)
	}
}

func handleSliceConfig(nwSlice *NetworkSlice, upf *upf) {
	log.Infoln("handle slice config : ", nwSlice.SliceName)

	ulMbr := calculateBitRates(nwSlice.SliceQos.UplinkMbr,
		nwSlice.SliceQos.BitrateUnit)
	dlMbr := calculateBitRates(nwSlice.SliceQos.DownlinkMbr,
		nwSlice.SliceQos.BitrateUnit)
	sliceInfo := SliceInfo{
		name:         nwSlice.SliceName,
		uplinkMbr:    ulMbr,
		downlinkMbr:  dlMbr,
		ulBurstBytes: nwSlice.SliceQos.UlBurstBytes,
		dlBurstBytes: nwSlice.SliceQos.DlBurstBytes,
	}

	if len(nwSlice.UeResInfo) > 0 {
		sliceInfo.ueResList = make([]UeResource, 0)

		for _, ueRes := range nwSlice.UeResInfo {
			var ueResInfo UeResource
			ueResInfo.dnn = ueRes.Dnn
			ueResInfo.name = ueRes.Name
			sliceInfo.ueResList = append(sliceInfo.ueResList, ueResInfo)
		}
	}

	err := upf.addSliceInfo(&sliceInfo)
	if err != nil {
		log.Errorln("adding slice info to datapath failed : ", err)
	}
}
