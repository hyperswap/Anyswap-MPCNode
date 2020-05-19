/*
 *  Copyright (C) 2018-2019  Fusion Foundation Ltd. All rights reserved.
 *  Copyright (C) 2018-2019  caihaijun@fusion.org
 *
 *  This library is free software; you can redistribute it and/or
 *  modify it under the Apache License, Version 2.0.
 *
 *  This library is distributed in the hope that it will be useful,
 *  but WITHOUT ANY WARRANTY; without even the implied warranty of
 *  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.
 *
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package dcrm 

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/fsn-dev/dcrm-walletService/mpcdsa/crypto/ec2"
	"github.com/fsn-dev/dcrm-walletService/mpcdsa/ecdsa/keygen"
	"github.com/fsn-dev/dcrm-walletService/crypto/secp256k1"
	"github.com/fsn-dev/dcrm-walletService/internal/common"
)

func GetReShareNonce(account string) (string, string, error) {
	key := Keccak256Hash([]byte(strings.ToLower(account + ":" + "RESHARE"))).Hex()
	exsit,da := GetValueFromPubKeyData(key)
	///////
	if exsit == false {
		return "0", "", nil
	}

	nonce, _ := new(big.Int).SetString(string(da.([]byte)), 10)
	one, _ := new(big.Int).SetString("1", 10)
	nonce = new(big.Int).Add(nonce, one)
	return fmt.Sprintf("%v", nonce), "", nil
}

func SetReShareNonce(account string,nonce string) (string, error) {
	key2 := Keccak256Hash([]byte(strings.ToLower(account + ":" + "RESHARE"))).Hex()
	kd := KeyData{Key: []byte(key2), Data: nonce}
	PubKeyDataChan <- kd
	LdbPubKeyData.WriteMap(key2, []byte(nonce))

	return "", nil
}

func reshare(wsid string, account string, pubkey string, nonce string,ch chan interface{}) {

	dcrmpks, _ := hex.DecodeString(pubkey)
	exsit,da := GetValueFromPubKeyData(string(dcrmpks[:]))
	///////
	if exsit == false {
		res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get reshare data from db fail", Err: fmt.Errorf("get reshare data from db fail")}
		ch <- res
		return
	}

	_,ok := da.(*PubKeyData)
	if ok == false {
		res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get reshare data from db fail", Err: fmt.Errorf("get reshare data from db fail")}
		ch <- res
		return
	}

	save := (da.(*PubKeyData)).Save
	rch := make(chan interface{}, 1)
	dcrm_reshare(wsid,save,rch)
	ret, _, cherr := GetChannelValue(ch_t, rch)
	if ret != "" {
		w, err := FindWorker(wsid)
		if w == nil || err != nil {
			res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:no find worker", Err: fmt.Errorf("get worker error.")}
			ch <- res
			return
		}

		///////TODO tmp
		//sid-enode:SendReShareRes:Success:ret
		//sid-enode:SendReShareRes:Fail:err
		mp := []string{w.sid, cur_enode}
		enode := strings.Join(mp, "-")
		s0 := "SendReShareRes"
		s1 := "Success"
		s2 := ret
		ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2
		SendMsgToDcrmGroup(ss, w.groupid)
		///////////////

		tip, reply := AcceptReShare("",account, w.groupid, nonce, pubkey, w.limitnum, "true", "true", "Success", ret, "", "", nil, w.id)
		if reply != nil {
			res := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("update reshare status error.")}
			ch <- res
			return
		}

		fmt.Printf("%v ================reshare,the terminal res is success. nonce =%v ==================\n", common.CurrentTime(), nonce)
		res := RpcDcrmRes{Ret: ret, Tip: tip, Err: err}
		ch <- res
		return
	}

	if cherr != nil {
		res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:reshare fail", Err: cherr}
		ch <- res
		return
	}

	return
}

//ec2
//msgprex = hash
//return value is the backup for dcrm sig.
func dcrm_reshare(msgprex string, save string,ch chan interface{}) {

	w, err := FindWorker(msgprex)
	if w == nil || err != nil {
		res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:no find worker", Err: fmt.Errorf("no find worker.")}
		ch <- res
		return
	}
	id := w.id

    var ch1 = make(chan interface{}, 1)
    for i:=0;i < recalc_times;i++ {
	if len(ch1) != 0 {
	    <-ch1
	}

	ReShare_ec2(msgprex, save, ch1, id)
	ret, _, cherr := GetChannelValue(ch_t, ch1)
	if ret != "" && cherr == nil {
		res := RpcDcrmRes{Ret: ret, Tip: "", Err: cherr}
		ch <- res
		break
	}
	
	w.Clear2()
	time.Sleep(time.Duration(3) * time.Second) //1000 == 1s
    }
}

//msgprex = hash
//return value is the backup for the dcrm sig
func ReShare_ec2(msgprex string, save string, ch chan interface{}, id int) {
	if id < 0 || id >= len(workers) {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("no find worker.")}
		ch <- res
		return
	}
	w := workers[id]
	if w.groupid == "" {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get group id fail.")}
		ch <- res
		return
	}

	mm := strings.Split(save, common.SepSave)
	if len(mm) == 0 {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get save data fail")}
		ch <- res
		return
	}

	// [Notes]
	// 1. assume the nodes who take part in the signature generation as follows
	ids := GetIds("ALL", w.groupid)
	idSign := ids[:w.ThresHold]

	//*******************!!!Distributed ECDSA Sign Start!!!**********************************

	skU1, w1 := MapPrivKeyShare("ALL", w, idSign, mm[0])
	if skU1 == nil || w1 == nil {
	    ////////test reshare///////////////////////
	    fmt.Printf("%v =============Sign_ec2,cur node not take part in reshare,key = %v ================\n", common.CurrentTime(), msgprex)
	    key := Keccak256Hash([]byte(strings.ToLower(w.DcrmFrom))).Hex()
	    exsit,da := GetValueFromPubKeyData(key)
	    if exsit == false {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get save data fail")}
		ch <- res
		return
	    }

	    pubs,ok := da.(*PubKeyData)
	    if ok == false {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get save data fail")}
		ch <- res
		return
	    }

	    ids = GetIds("ALL", pubs.GroupId)
	    
	    _, tip, cherr := GetChannelValue(ch_t, w.bc11)
	    suss := false
	    if cherr != nil {
		suss = ReqDataFromGroup(msgprex,w.id,"C11",reqdata_trytimes,reqdata_timeout)
	    } else {
		suss = true
	    }
	    if !suss {
		    res := RpcDcrmRes{Ret: "", Tip: tip, Err: GetRetErr(ErrGetC11Timeout)}
		    ch <- res
		    return
	    }

	    _, tip, cherr = GetChannelValue(ch_t, w.bss1)
	    suss = false
	    if cherr != nil {
		suss = ReqDataFromGroup(msgprex,w.id,"SS1",reqdata_trytimes,reqdata_timeout)
	    } else {
		suss = true
	    }
	    if !suss {
		    res := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("get ss1 timeout")}
		    ch <- res
		    return
	    }

	    _, tip, cherr = GetChannelValue(ch_t, w.bd11_1)
	    suss = false
	    if cherr != nil {
		suss = ReqDataFromGroup(msgprex,w.id,"D11",reqdata_trytimes,reqdata_timeout)
	    } else {
		suss = true
	    }
	    if !suss {
		    res := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("get d11 timeout")}
		    ch <- res
		    return
	    }

	    ss1s2 := make([]string, w.ThresHold)
	    if w.msg_ss1.Len() != w.ThresHold {
		    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get all ss1 fail.")}
		    ch <- res
		    return
	    }

	    itmp := 0
	    iter := w.msg_ss1.Front()
	    for iter != nil {
		    mdss := iter.Value.(string)
		    ss1s2[itmp] = mdss
		    iter = iter.Next()
		    itmp++
	    }

	    c11s := make([]string, w.ThresHold)
	    if w.msg_c11.Len() != w.ThresHold {
		    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get all c11 fail.")}
		    ch <- res
		    return
	    }

	    itmp = 0
	    iter = w.msg_c11.Front()
	    for iter != nil {
		    mdss := iter.Value.(string)
		    c11s[itmp] = mdss
		    iter = iter.Next()
		    itmp++
	    }

	    // for all nodes, construct the commitment by the receiving C and D
	    var udecom = make(map[string]*ec2.Commitment)
	    for _, v := range c11s {
		    mm := strings.Split(v, common.Sep)
		    if len(mm) < 3 {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_c11 fail.")}
			    ch <- res
			    return
		    }

		    prex := mm[0]
		    prexs := strings.Split(prex, "-")
		    for _, vv := range ss1s2 {
			    mmm := strings.Split(vv, common.Sep)
			    if len(mmm) < 3 {
				    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 fail.")}
				    ch <- res
				    return
			    }

			    prex2 := mmm[0]
			    prexs2 := strings.Split(prex2, "-")
			    if prexs[len(prexs)-1] == prexs2[len(prexs2)-1] {
				    dlen, _ := strconv.Atoi(mmm[2])
				    var gg = make([]*big.Int, 0)
				    l := 0
				    for j := 0; j < dlen; j++ {
					    l++
					    if len(mmm) < (3 + l) {
						    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 fail.")}
						    ch <- res
						    return 
					    }

					    gg = append(gg, new(big.Int).SetBytes([]byte(mmm[2+l])))
				    }

				    deCommit := &ec2.Commitment{C: new(big.Int).SetBytes([]byte(mm[2])), D: gg}
				    udecom[prexs[len(prexs)-1]] = deCommit
				    break
			    }
		    }
	    }

	    // for all nodes, verify the commitment
	    for _, id := range idSign {
		    enodes := GetEnodesByUid(id, "ALL", w.groupid)
		    ////////bug
		    if len(enodes) < 9 {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get enodes error")}
			    ch <- res
			    return 
		    }
		    ////////
		    en := strings.Split(string(enodes[8:]), "@")
		    //bug
		    if len(en) <= 0 || en[0] == "" {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			    ch <- res
			    return
		    }

		    _, exsit := udecom[en[0]]
		    if exsit == false {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			    ch <- res
			    return
		    }
		    //

		    if udecom[en[0]] == nil {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			    ch <- res
			    return
		    }

		    if keygen.DECDSA_Key_Commitment_Verify(udecom[en[0]]) == false {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			    ch <- res
			    return
		    }
	    }

	    var sstruct = make(map[string]*ec2.ShareStruct2)
	    shares := make([]string, w.ThresHold)
	    if w.msg_d11_1.Len() != w.ThresHold {
		    res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrGetAllSHARE1Fail)}
		    ch <- res
		    return
	    }

	    itmp = 0
	    iter = w.msg_d11_1.Front()
	    for iter != nil {
		    mdss := iter.Value.(string)
		    shares[itmp] = mdss
		    iter = iter.Next()
		    itmp++
	    }

	    for _, v := range shares {
		    mm := strings.Split(v, common.Sep)
		    //bug
		    if len(mm) < 4 {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("fill ec2.ShareStruct map error.")}
			    ch <- res
			    return
		    }
		    //
		    ushare := &ec2.ShareStruct2{Id: new(big.Int).SetBytes([]byte(mm[2])), Share: new(big.Int).SetBytes([]byte(mm[3]))}
		    prex := mm[0]
		    prexs := strings.Split(prex, "-")
		    sstruct[prexs[len(prexs)-1]] = ushare
	    }

	    var upg = make(map[string]*ec2.PolyGStruct2)
	    for _, v := range ss1s2 {
		    mm := strings.Split(v, common.Sep)
		    dlen, _ := strconv.Atoi(mm[2])
		    if len(mm) < (4 + dlen) {
			    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
			    ch <- res
			    return 
		    }

		    pglen, _ := strconv.Atoi(mm[3+dlen])
		    pglen = (pglen / 2)
		    var pgss = make([][]*big.Int, 0)
		    l := 0
		    for j := 0; j < pglen; j++ {
			    l++
			    var gg = make([]*big.Int, 0)
			    if len(mm) < (4 + dlen + l) {
				    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
				    ch <- res
				    return
			    }

			    gg = append(gg, new(big.Int).SetBytes([]byte(mm[3+dlen+l])))
			    l++
			    if len(mm) < (4 + dlen + l) {
				    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
				    ch <- res
				    return
			    }
			    gg = append(gg, new(big.Int).SetBytes([]byte(mm[3+dlen+l])))
			    pgss = append(pgss, gg)
		    }

		    ps := &ec2.PolyGStruct2{PolyG: pgss}
		    prex := mm[0]
		    prexs := strings.Split(prex, "-")
		    upg[prexs[len(prexs)-1]] = ps
	    }

	    // 3. verify the share
	    for _, id := range idSign {
		    enodes := GetEnodesByUid(id, "ALL", w.groupid)
		    en := strings.Split(string(enodes[8:]), "@")
		    //bug
		    if len(en) == 0 || en[0] == "" || sstruct[en[0]] == nil || upg[en[0]] == nil {
			    res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrVerifySHARE1Fail)}
			    ch <- res
			    return 
		    }
		    //
		    if keygen.DECDSA_Key_Verify_Share(sstruct[en[0]], upg[en[0]]) == false {
			    res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrVerifySHARE1Fail)}
			    ch <- res
			    return
		    }
	    }

	    var newskU1 *big.Int
	    for _, id := range idSign {
		    enodes := GetEnodesByUid(id, "ALL", w.groupid)
		    en := strings.Split(string(enodes[8:]), "@")
		    newskU1 = sstruct[en[0]].Share
		    break
	    }

	    for k, id := range idSign {
		    if k == 0 {
			    continue
		    }

		    enodes := GetEnodesByUid(id, "ALL", w.groupid)
		    en := strings.Split(string(enodes[8:]), "@")
		    newskU1 = new(big.Int).Add(newskU1, sstruct[en[0]].Share)
	    }
	    newskU1 = new(big.Int).Mod(newskU1, secp256k1.S256().N)
	    res := RpcDcrmRes{Ret: fmt.Sprintf("%v",newskU1), Err: nil}
	    ch <- res
	    return
	}

	/////////test reshare //////////
	skP1Poly, skP1PolyG, _ := ec2.Vss2Init(w1, w.ThresHold)
	skP1Gx, skP1Gy := secp256k1.S256().ScalarBaseMult(w1.Bytes())
	u1CommitValues := make([]*big.Int, 0)
	u1CommitValues = append(u1CommitValues, skP1Gx)
	u1CommitValues = append(u1CommitValues, skP1Gy)
	for i := 1; i < len(skP1PolyG.PolyG); i++ {
		u1CommitValues = append(u1CommitValues, skP1PolyG.PolyG[i][0])
		u1CommitValues = append(u1CommitValues, skP1PolyG.PolyG[i][1])
	}
	commitSkP1G := new(ec2.Commitment).Commit(u1CommitValues...)

	key := Keccak256Hash([]byte(strings.ToLower(w.DcrmFrom))).Hex()
	exsit,da := GetValueFromPubKeyData(key)
	if exsit == false {
	    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get save data fail")}
	    ch <- res
	    return
	}

	pubs,ok := da.(*PubKeyData)
	if ok == false {
	    res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get save data fail")}
	    ch <- res
	    return
	}

	ids = GetIds("ALL", pubs.GroupId)

	mp := []string{msgprex, cur_enode}
	enode := strings.Join(mp, "-")
	s0 := "C11"
	s1 := string(commitSkP1G.C.Bytes())
	ss := enode + common.Sep + s0 + common.Sep + s1
	SendMsgToDcrmGroup(ss, pubs.GroupId)
	DisMsg(ss)

	_, tip, cherr := GetChannelValue(ch_t, w.bc11)
	suss := false
	if cherr != nil {
	    suss = ReqDataFromGroup(msgprex,w.id,"C11",reqdata_trytimes,reqdata_timeout)
	} else {
	    suss = true
	}
	if !suss {
		res := RpcDcrmRes{Ret: "", Tip: tip, Err: GetRetErr(ErrGetC11Timeout)}
		ch <- res
		return 
	}

	skP1Shares, err := keygen.DECDSA_Key_Vss(skP1Poly, ids)
	if err != nil {
		res := RpcDcrmRes{Ret: "", Err: err}
		ch <- res
		return 
	}

	for _, id := range ids {
		enodes := GetEnodesByUid(id, "ALL", pubs.GroupId)

		if enodes == "" {
			res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrGetEnodeByUIdFail)}
			ch <- res
			return 
		}

		if IsCurNode(enodes, cur_enode) {
			continue
		}

		for _, v := range skP1Shares {
			uid := keygen.DECDSA_Key_GetSharesId(v)
			if uid != nil && uid.Cmp(id) == 0 {
				mp := []string{msgprex, cur_enode}
				enode := strings.Join(mp, "-")
				s0 := "D11"
				s2 := string(v.Id.Bytes())
				s3 := string(v.Share.Bytes())
				ss := enode + common.Sep + s0 + common.Sep + s2 + common.Sep + s3
				SendMsgToPeer(enodes, ss)
				break
			}
		}
	}

	for _, v := range skP1Shares {
		uid := keygen.DECDSA_Key_GetSharesId(v)
		if uid == nil {
			continue
		}

		enodes := GetEnodesByUid(uid, "ALL", pubs.GroupId)
		if IsCurNode(enodes, cur_enode) {
			mp := []string{msgprex, cur_enode}
			enode := strings.Join(mp, "-")
			s0 := "D11"
			s2 := string(v.Id.Bytes())
			s3 := string(v.Share.Bytes())
			ss := enode + common.Sep + s0 + common.Sep + s2 + common.Sep + s3
			DisMsg(ss)
			break
		}
	}

	mp = []string{msgprex, cur_enode}
	enode = strings.Join(mp, "-")
	s0 = "SS1"
	dlen := len(commitSkP1G.D)
	s1 = strconv.Itoa(dlen)

	ss = enode + common.Sep + s0 + common.Sep + s1 + common.Sep
	for _, d := range commitSkP1G.D {
		ss += string(d.Bytes())
		ss += common.Sep
	}

	pglen := 2 * (len(skP1PolyG.PolyG))
	s4 := strconv.Itoa(pglen)

	ss = ss + s4 + common.Sep

	for _, p := range skP1PolyG.PolyG {
		for _, d := range p {
			ss += string(d.Bytes())
			ss += common.Sep
		}
	}
	ss = ss + "NULL"
	SendMsgToDcrmGroup(ss, pubs.GroupId)
	DisMsg(ss)
	
	_, tip, cherr = GetChannelValue(ch_t, w.bss1)
	suss = false
	if cherr != nil {
	    suss = ReqDataFromGroup(msgprex,w.id,"SS1",reqdata_trytimes,reqdata_timeout)
	} else {
	    suss = true
	}
	if !suss {
		res := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("get ss1 timeout")}
		ch <- res
		return
	}

	_, tip, cherr = GetChannelValue(ch_t, w.bd11_1)
	suss = false
	if cherr != nil {
	    suss = ReqDataFromGroup(msgprex,w.id,"D11",reqdata_trytimes,reqdata_timeout)
	} else {
	    suss = true
	}
	if !suss {
		res := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("get d11 timeout")}
		ch <- res
		return 
	}

	ss1s2 := make([]string, w.ThresHold)
	if w.msg_ss1.Len() != w.ThresHold {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get all ss1 fail.")}
		ch <- res
		return
	}

	itmp := 0
	iter := w.msg_ss1.Front()
	for iter != nil {
		mdss := iter.Value.(string)
		ss1s2[itmp] = mdss
		iter = iter.Next()
		itmp++
	}

	c11s := make([]string, w.ThresHold)
	if w.msg_c11.Len() != w.ThresHold {
		res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get all c11 fail.")}
		ch <- res
		return
	}

	itmp = 0
	iter = w.msg_c11.Front()
	for iter != nil {
		mdss := iter.Value.(string)
		c11s[itmp] = mdss
		iter = iter.Next()
		itmp++
	}

	// for all nodes, construct the commitment by the receiving C and D
	var udecom = make(map[string]*ec2.Commitment)
	for _, v := range c11s {
		mm := strings.Split(v, common.Sep)
		if len(mm) < 3 {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_c11 fail.")}
			ch <- res
			return
		}

		prex := mm[0]
		prexs := strings.Split(prex, "-")
		for _, vv := range ss1s2 {
			mmm := strings.Split(vv, common.Sep)
			if len(mmm) < 3 {
				res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 fail.")}
				ch <- res
				return
			}

			prex2 := mmm[0]
			prexs2 := strings.Split(prex2, "-")
			if prexs[len(prexs)-1] == prexs2[len(prexs2)-1] {
				dlen, _ := strconv.Atoi(mmm[2])
				var gg = make([]*big.Int, 0)
				l := 0
				for j := 0; j < dlen; j++ {
					l++
					if len(mmm) < (3 + l) {
						res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 fail.")}
						ch <- res
						return
					}

					gg = append(gg, new(big.Int).SetBytes([]byte(mmm[2+l])))
				}

				deCommit := &ec2.Commitment{C: new(big.Int).SetBytes([]byte(mm[2])), D: gg}
				udecom[prexs[len(prexs)-1]] = deCommit
				break
			}
		}
	}

	// for all nodes, verify the commitment
	for _, id := range idSign {
		enodes := GetEnodesByUid(id, "ALL", w.groupid)
		////////bug
		if len(enodes) < 9 {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get enodes error")}
			ch <- res
			return 
		}
		////////
		en := strings.Split(string(enodes[8:]), "@")
		//bug
		if len(en) <= 0 || en[0] == "" {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			ch <- res
			return 
		}

		_, exsit := udecom[en[0]]
		if exsit == false {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			ch <- res
			return 
		}
		//

		if udecom[en[0]] == nil {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			ch <- res
			return 
		}

		if keygen.DECDSA_Key_Commitment_Verify(udecom[en[0]]) == false {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("verify commit fail.")}
			ch <- res
			return 
		}
	}

	var sstruct = make(map[string]*ec2.ShareStruct2)
	shares := make([]string, w.ThresHold)
	if w.msg_d11_1.Len() != w.ThresHold {
		res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrGetAllSHARE1Fail)}
		ch <- res
		return
	}

	itmp = 0
	iter = w.msg_d11_1.Front()
	for iter != nil {
		mdss := iter.Value.(string)
		shares[itmp] = mdss
		iter = iter.Next()
		itmp++
	}

	for _, v := range shares {
		mm := strings.Split(v, common.Sep)
		//bug
		if len(mm) < 4 {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("fill ec2.ShareStruct map error.")}
			ch <- res
			return 
		}
		//
		ushare := &ec2.ShareStruct2{Id: new(big.Int).SetBytes([]byte(mm[2])), Share: new(big.Int).SetBytes([]byte(mm[3]))}
		prex := mm[0]
		prexs := strings.Split(prex, "-")
		sstruct[prexs[len(prexs)-1]] = ushare
	}

	var upg = make(map[string]*ec2.PolyGStruct2)
	for _, v := range ss1s2 {
		mm := strings.Split(v, common.Sep)
		dlen, _ := strconv.Atoi(mm[2])
		if len(mm) < (4 + dlen) {
			res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
			ch <- res
			return 
		}

		pglen, _ := strconv.Atoi(mm[3+dlen])
		pglen = (pglen / 2)
		var pgss = make([][]*big.Int, 0)
		l := 0
		for j := 0; j < pglen; j++ {
			l++
			var gg = make([]*big.Int, 0)
			if len(mm) < (4 + dlen + l) {
				res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
				ch <- res
				return 
			}

			gg = append(gg, new(big.Int).SetBytes([]byte(mm[3+dlen+l])))
			l++
			if len(mm) < (4 + dlen + l) {
				res := RpcDcrmRes{Ret: "", Err: fmt.Errorf("get msg_ss1 data error")}
				ch <- res
				return 
			}
			gg = append(gg, new(big.Int).SetBytes([]byte(mm[3+dlen+l])))
			pgss = append(pgss, gg)
		}

		ps := &ec2.PolyGStruct2{PolyG: pgss}
		prex := mm[0]
		prexs := strings.Split(prex, "-")
		upg[prexs[len(prexs)-1]] = ps
	}

	// 3. verify the share
	for _, id := range idSign {
		enodes := GetEnodesByUid(id, "ALL", w.groupid)
		en := strings.Split(string(enodes[8:]), "@")
		//bug
		if len(en) == 0 || en[0] == "" || sstruct[en[0]] == nil || upg[en[0]] == nil {
			res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrVerifySHARE1Fail)}
			ch <- res
			return
		}
		//
		if keygen.DECDSA_Key_Verify_Share(sstruct[en[0]], upg[en[0]]) == false {
			res := RpcDcrmRes{Ret: "", Err: GetRetErr(ErrVerifySHARE1Fail)}
			ch <- res
			return 
		}
	}

	var newskU1 *big.Int
	for _, id := range idSign {
		enodes := GetEnodesByUid(id, "ALL", w.groupid)
		en := strings.Split(string(enodes[8:]), "@")
		newskU1 = sstruct[en[0]].Share
		break
	}

	for k, id := range idSign {
		if k == 0 {
			continue
		}

		enodes := GetEnodesByUid(id, "ALL", w.groupid)
		en := strings.Split(string(enodes[8:]), "@")
		newskU1 = new(big.Int).Add(newskU1, sstruct[en[0]].Share)
	}
	newskU1 = new(big.Int).Mod(newskU1, secp256k1.S256().N)
	res := RpcDcrmRes{Ret: fmt.Sprintf("%v",newskU1), Err: nil}
	ch <- res
	////////////////////////////////
}
