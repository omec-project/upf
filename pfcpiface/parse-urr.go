// SPDX-License-Identifier: Apache-2.0
// Copyright(c) 2020 Intel Corporation

package main

import (
	"log"

	"github.com/wmnsk/go-pfcp/ie"
)

type reportRecord struct {
	fseid uint64
	srcIP string
	urrs  *[]urr
}

func (r *reportRecord) checkInvalid() bool {
	var flag bool = true
	for _, urr := range *r.urrs {
		if urr.reportOpen {
			flag = false
			break
		}
	}
	return flag
}

type volumeData struct {
	flags       uint8
	totalVol    uint64
	uplinkVol   uint64
	downlinkVol uint64
}

type reportTrigger struct {
	flags uint16
}

func (r *reportTrigger) isVOLTHSet() bool {
	u8 := uint8(r.flags >> 8)
	return has2ndBit(u8)
}
func (r *reportTrigger) isVOLQUSet() bool {
	u8 := uint8(r.flags)
	return has1stBit(u8)
}

type urr struct {
	urrID      uint32
	ctrID      uint32
	pdrID      uint32
	measureM   uint8
	reportOpen bool
	reportT    reportTrigger
	//measureP   uint32
	localVol    uint64
	localThresh uint64
	volThresh   volumeData
	volQuota    volumeData
}

/*
func (u *urr) printURR() {
	log.Println("------------------ URR ---------------------")
	log.Println("URR ID:", u.urrID)
	log.Println("CTR ID:", u.ctrID)
	log.Println("PDR ID:", u.pdrID)
	log.Println("Measurement Method:", u.measureM)
	log.Println("Report Trigger:", u.reportT.flags)
    log.Println("Total Volume Threshold:", u.volThresh.totalVol)
    log.Println("Total Volume Quota:", u.volQuota.totalVol)
	log.Println("--------------------------------------------")
}*/

func (u *urr) parseURR(ie1 *ie.IE, seid uint64) error {
	log.Println("Parse Create URR")
	volumeThresh := volumeData{}
	volumeQuota := volumeData{}

	urrID, err := ie1.URRID()
	if err != nil {
		log.Println("Could not read urrID!")
		return err
	}

	measureMeth, err := ie1.MeasurementMethod()
	if err != nil {
		log.Println("Could not read Measurement method!")
		return err
	}

	trigger, err := ie1.ReportingTriggers()
	if err != nil {
		log.Println("Could not read Reporting triggers!")
		return err
	}

	reportTrig := reportTrigger{flags: trigger}
	volThreshField, err := ie1.VolumeThreshold()
	if err == nil {
		volumeThresh.flags = volThreshField.Flags
		volumeThresh.totalVol = volThreshField.TotalVolume
		volumeThresh.uplinkVol = volThreshField.UplinkVolume
		volumeThresh.downlinkVol = volThreshField.DownlinkVolume
	} else {
		log.Println("VolumeThreshold IE read failed")
		return err
	}

	volQuotaField, err := ie1.VolumeQuota()
	if err == nil {
		volumeQuota.flags = volQuotaField.Flags
		volumeQuota.totalVol = volQuotaField.TotalVolume
		volumeQuota.uplinkVol = volQuotaField.UplinkVolume
		volumeQuota.downlinkVol = volQuotaField.DownlinkVolume
	} else {
		log.Println("VolumeQuota IE read failed")
		return err
	}

	u.urrID = uint32(urrID)
	u.measureM = measureMeth
	u.reportT = reportTrig
	u.reportOpen = true
	u.volThresh = volumeThresh
	u.localThresh = volumeThresh.totalVol
	u.volQuota = volumeQuota

	return nil
}
