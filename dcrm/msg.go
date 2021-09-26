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
	"github.com/fsn-dev/dcrm-walletService/internal/common"
	"github.com/fsn-dev/dcrm-walletService/crypto/secp256k1"
	"strings"
	"math/big"
	"encoding/hex"
	"fmt"
	"time"
	"container/list"
	"github.com/fsn-dev/cryptoCoins/coins"
	"crypto/ecdsa"
	"github.com/fsn-dev/dcrm-walletService/crypto"
	"github.com/fsn-dev/dcrm-walletService/crypto/ecies"
	"strconv"
	"github.com/fsn-dev/dcrm-walletService/p2p/discover"
	crand "crypto/rand"
	"github.com/fsn-dev/cryptoCoins/coins/types"
	"github.com/fsn-dev/cryptoCoins/tools/rlp"
	"encoding/json"
	"runtime/debug"
)

var (
	C1Data  = common.NewSafeMap(10)
	ch_t                     = 120 
	WaitMsgTimeGG20                     = 100
	waitall                     = ch_t * recalc_times
	waitallgg20                     = WaitMsgTimeGG20 * recalc_times
	AgreeWait = 2

	syncpresign = true 
	
	//callback
	GetGroup               func(string) (int, string)
	SendToGroupAllNodes    func(string, string) (string, error)
	GetSelfEnode           func() string
	BroadcastInGroupOthers func(string, string) (string, error)
	SendToPeer             func(string, string) error
	ParseNode              func(string) string
	GetEosAccount          func() (string, string, string)
)

//p2p callback
func RegP2pGetGroupCallBack(f func(string) (int, string)) {
	GetGroup = f
}

func RegP2pSendToGroupAllNodesCallBack(f func(string, string) (string, error)) {
	SendToGroupAllNodes = f
}

func RegP2pGetSelfEnodeCallBack(f func() string) {
	GetSelfEnode = f
}

func RegP2pBroadcastInGroupOthersCallBack(f func(string, string) (string, error)) {
	BroadcastInGroupOthers = f
}

func RegP2pSendMsgToPeerCallBack(f func(string, string) error) {
	SendToPeer = f
}

func RegP2pParseNodeCallBack(f func(string) string) {
	ParseNode = f
}

func RegDcrmGetEosAccountCallBack(f func() (string, string, string)) {
	GetEosAccount = f
}

////////////////////////////////

func SendMsgToDcrmGroup(msg string, groupid string) {
	common.Debug("=========SendMsgToDcrmGroup=============","msg",msg,"groupid",groupid)
	_,err := BroadcastInGroupOthers(groupid, msg)
	if err != nil {
	    common.Debug("=========SendMsgToDcrmGroup,send msg to dcrm group=============","msg",msg,"groupid",groupid,"err",err)
	}
}

///
func EncryptMsg(msg string, enodeID string) (string, error) {
	//fmt.Println("=============EncryptMsg,KeyFile = %s,enodeID = %s ================",KeyFile,enodeID)
	hprv, err1 := hex.DecodeString(enodeID)
	if err1 != nil {
		return "", err1
	}

	//fmt.Println("=============EncryptMsg,hprv len = %v ================",len(hprv))
	p := &ecdsa.PublicKey{Curve: crypto.S256(), X: new(big.Int), Y: new(big.Int)}
	half := len(hprv) / 2
	p.X.SetBytes(hprv[:half])
	p.Y.SetBytes(hprv[half:])
	if !p.Curve.IsOnCurve(p.X, p.Y) {
		return "", fmt.Errorf("id is invalid secp256k1 curve point")
	}

	var cm []byte
	pub := ecies.ImportECDSAPublic(p)
	cm, err := ecies.Encrypt(crand.Reader, pub, []byte(msg), nil, nil)
	if err != nil {
		return "", err
	}

	return string(cm), nil
}

func DecryptMsg(cm string) (string, error) {
	//test := Keccak256Hash([]byte(strings.ToLower(cm))).Hex()
	nodeKey, errkey := crypto.LoadECDSA(KeyFile)
	if errkey != nil {
		//fmt.Printf("%v =========DecryptMsg finish crypto.LoadECDSA,err = %v,keyfile = %v,msg hash = %v =================\n", common.CurrentTime(), errkey, KeyFile, test)
		return "", errkey
	}

	prv := ecies.ImportECDSA(nodeKey)
	var m []byte
	m, err := prv.Decrypt([]byte(cm), nil, nil)
	if err != nil {
		//fmt.Printf("%v =========DecryptMsg finish prv.Decrypt,err = %v,keyfile = %v,msg hash = %v =================\n", common.CurrentTime(), err, KeyFile, test)
		return "", err
	}

	return string(m), nil
}

///
func SendMsgToPeer(enodes string, msg string) {
//	common.Debug("=========SendMsgToPeer===========","msg",msg,"send to peer",enodes)
	en := strings.Split(string(enodes[8:]), "@")
	cm, err := EncryptMsg(msg, en[0])
	if err != nil {
		//fmt.Printf("%v =========SendMsgToPeer,encrypt msg fail,err = %v =================\n", common.CurrentTime(), err)
		return
	}

	err = SendToPeer(enodes, cm)
	if err != nil {
//	    common.Debug("=========SendMsgToPeer,send to peer fail===========","msg",msg,"send to peer",enodes,"err",err)
	    return
	}
}

type RawReply struct {
    From string
    Accept string
    TimeStamp string
}

func GetRawReply(l *list.List) map[string]*RawReply {
    ret := make(map[string]*RawReply)
    if l == nil {
	return ret
    }

    var next *list.Element
    for e := l.Front(); e != nil; e = next {
	next = e.Next()

	if e.Value == nil {
		continue
	}

	s := e.Value.(string)

	if s == "" {
		continue
	}

	raw := s 
	_,from,_,txdata,err := CheckRaw(raw)
	if err != nil {
	    continue
	}
	
	req,ok := txdata.(*TxDataReqAddr)
	if ok {
	    reply := &RawReply{From:from,Accept:"true",TimeStamp:req.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}

	    }

	    continue
	}
	
	sig,ok := txdata.(*TxDataSign)
	if ok {
	    //common.Debug("=================GetRawReply,item is sign cmd data=================","key",keytmp,"from",from,"sig",sig)
	    reply := &RawReply{From:from,Accept:"true",TimeStamp:sig.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}
	    }

	    continue
	}
	
	rh,ok := txdata.(*TxDataReShare)
	if ok {
	    reply := &RawReply{From:from,Accept:"true",TimeStamp:rh.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}
	    }

	    continue
	}
	
	acceptreq,ok := txdata.(*TxDataAcceptReqAddr)
	if ok {
	    accept := "false"
	    if acceptreq.Accept == "AGREE" {
		    accept = "true"
	    }

	    reply := &RawReply{From:from,Accept:accept,TimeStamp:acceptreq.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}

	    }
	}
	
	acceptsig,ok := txdata.(*TxDataAcceptSign)
	if ok {
	    //common.Debug("=================GetRawReply,item is sign accept data================","key",keytmp,"from",from,"accept",acceptsig.Accept,"raw",raw)
	    accept := "false"
	    if acceptsig.Accept == "AGREE" {
		    accept = "true"
	    }

	    reply := &RawReply{From:from,Accept:accept,TimeStamp:acceptsig.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}

	    }
	}
	
	acceptrh,ok := txdata.(*TxDataAcceptReShare)
	if ok {
	    accept := "false"
	    if acceptrh.Accept == "AGREE" {
		    accept = "true"
	    }

	    reply := &RawReply{From:from,Accept:accept,TimeStamp:acceptrh.TimeStamp}
	    tmp,ok := ret[from]
	    if !ok {
		ret[from] = reply
	    } else {
		t1,_ := new(big.Int).SetString(reply.TimeStamp,10)
		t2,_ := new(big.Int).SetString(tmp.TimeStamp,10)
		if t1.Cmp(t2) > 0 {
		    ret[from] = reply
		}

	    }
	}
    }

    return ret
}

func CheckReply(l *list.List,rt RpcType,key string) bool {
    if l == nil || key == "" {
	return false
    }

    /////reshare only
    if rt == Rpc_RESHARE {
	exsit,da := GetReShareInfoData([]byte(key))
	if !exsit {
	    return false
	}

	ac,ok := da.(*AcceptReShareData)
	if !ok || ac == nil {
	    return false
	}

	ret := GetRawReply(l)
	_, enodes := GetGroup(ac.GroupId)
	nodes := strings.Split(enodes, common.Sep2)
	for _, node := range nodes {
	    node2 := ParseNode(node)
	    pk := "04" + node2 
	     h := coins.NewCryptocoinHandler("FSN")
	     if h == nil {
		continue
	     }

	    fr, err := h.PublicKeyToAddress(pk)
	    if err != nil {
		return false
	    }

	    found := false
	    for _,v := range ret {
		if strings.EqualFold(v.From,fr) {
		    found = true
		    break
		}
	    }

	    if !found {
		return false
	    }
	}

	return true
    }
    /////////////////

    k := ""
    if rt == Rpc_REQADDR {
	k = key
    } else {
	k = GetReqAddrKeyByOtherKey(key,rt)
    }

    if k == "" {
	return false
    }

    exsit,da := GetReqAddrInfoData([]byte(k))
    if !exsit || da == nil {
	exsit,da = GetValueFromDb(k)
    }
    if !exsit {
	return false
    }

    ac,ok := da.(*AcceptReqAddrData)
    if !ok {
	return false
    }

    if ac == nil {
	return false
    }

    ret := GetRawReply(l)

    if rt == Rpc_REQADDR {
	//sigs:  5:eid1:acc1:eid2:acc2:eid3:acc3:eid4:acc4:eid5:acc5
	mms := strings.Split(ac.Sigs, common.Sep)
	count := (len(mms) - 1)/2
	if count <= 0 {
	    common.Error("===================== CheckReply,reqaddr sigs data error.================","ac.Sigs",ac.Sigs,"count",count,"k",k,"key",key,"ret",ret)
	    return false
	}

	for j:=0;j<count;j++ {
	    found := false
	    for _,v := range ret {
		    //common.Debug("===================== CheckReply,reqaddr================","ac.Sigs",ac.Sigs,"count",count,"k",k,"key",key,"ret.v",v,"v.From",v.From,"mms[2j+2]",mms[2*j+2])
		if strings.EqualFold(v.From,mms[2*j+2]) { //allow user login diffrent node
		    found = true
		    break
		}
	    }

	    if !found {
		common.Error("===================== CheckReply, Not found.====================","ac.Sigs",ac.Sigs,"count",count,"k",k,"key",key)
		return false
	    }
	}

	return true
    }

    if rt == Rpc_SIGN {
	exsit,data := GetSignInfoData([]byte(key))
	if !exsit {
	    common.Error("===================== CheckReply,get sign data by key fail from local db================","key",key)
	    return false
	}

	sig,ok := data.(*AcceptSignData)
	if !ok || sig == nil {
	    common.Error("===================== CheckReply,get sign data by key error from local db================","key",key)
	    return false
	}

	mms := strings.Split(ac.Sigs, common.Sep)
	_, enodes := GetGroup(sig.GroupId)
	nodes := strings.Split(enodes, common.Sep2)
	for _, node := range nodes {
	    node2 := ParseNode(node)
	    foundeid := false
	    for kk,v := range mms {
		if strings.EqualFold(v,node2) {
		    foundeid = true
		    found := false
		    for _,vv := range ret {
			    //common.Debug("===================== CheckReply, mms[kk+1] must in ret map===============","key",key,"ret[...]",vv.From,"mms[kk+1]",mms[kk+1],"ac.Sigs",ac.Sigs)
			if strings.EqualFold(vv.From,mms[kk+1]) { //allow user login diffrent node
			    found = true
			    break
			}
		    }

		    if !found {
			//common.Debug("===================== CheckReply,mms[kk+1] no find in ret map and return fail==================","key",key,"mms[kk+1]",mms[kk+1])
			return false
		    }

		    break
		}
	    }

	    if !foundeid {
		//common.Debug("===================== CheckReply,find eid fail================","key",key)
		return false
	    }
	}

	return true
    }

    return false 
}

//=========================================

func Call(msg interface{}, enode string) {
	common.Debug("====================Call===================","get msg",msg,"sender node",enode)
	s := msg.(string)
	if s == "" {
	    return
	}

	raw,err := UnCompress(s)
	if err == nil {
		s = raw
	}
	
	SetUpMsgList(s, enode)
}

func SetUpMsgList(msg string, enode string) {

	v := RecvMsg{msg: msg, sender: enode}
	//rpc-req
	rch := make(chan interface{}, 1)
	req := RPCReq{rpcdata: &v, ch: rch}
	RPCReqQueue <- req
}

func SetUpMsgList3(msg string, enode string,rch chan interface{}) {

	v := RecvMsg{msg: msg, sender: enode}
	//rpc-req
	req := RPCReq{rpcdata: &v, ch: rch}
	RPCReqQueue <- req
}

//==================================================================

type WorkReq interface {
    Run(workid int, ch chan interface{}) bool
}

//RecvMsg
type RecvMsg struct {
	msg    string
	sender string
}

func SynchronizePreSignData(msgprex string,wid int,success bool) bool {
    w := workers[wid]
    if w == nil {
	return false
    }

    msg := "success"
    if !success {
	msg = "fail"
    }

    //key-enode:SyncPreSign:success
    mp := []string{msgprex, cur_enode}
    enode := strings.Join(mp, "-")
    s0 := "SyncPreSign"
    ss := enode + common.Sep + s0 + common.Sep + msg 
    SendMsgToDcrmGroup(ss,w.groupid)
    DisMsg(ss)

    reply := false
    timeout := make(chan bool, 1)
    go func() {
	syncWaitTime := 20 * time.Second
	syncWaitTimeOut := time.NewTicker(syncWaitTime)
	
	for {
	    select {
	    case <-w.bsyncpresign:
		iter := w.msg_syncpresign.Front()
		for iter != nil {
		    val := iter.Value.(string)
		    if val == "" {
			reply = false
			timeout <-false
			return
		    }

		    m := strings.Split(val,common.Sep)
		    if len(m) < 3 {
			reply = false
			timeout <-false
			return
		    }

		    if strings.EqualFold(m[2],"fail") {
			reply = false
			timeout <-false
			return
		    }

		    iter = iter.Next()
		}

		reply = true 
		timeout <-false
		return
	    case <-syncWaitTimeOut.C:
		reply = false
		timeout <-true
		return
	    }
	}
    }()

    <-timeout
    return reply
}

func (self *RecvMsg) Run(workid int, ch chan interface{}) bool {
	if workid < 0 || workid >= RPCMaxWorker {
		res2 := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get worker id fail", Err: fmt.Errorf("no find worker.")}
		ch <- res2
		return false
	}

	res := self.msg
	if res == "" {
		res2 := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get data fail in RecvMsg.Run", Err: fmt.Errorf("no find worker.")}
		ch <- res2
		return false
	}

	common.Debug("====================================RecvMsg.Run================================","get msg",res)
	msgdata, errdec := DecryptMsg(res) //for SendMsgToPeer
	if errdec == nil {
		res = msgdata
	}
	mm := strings.Split(res, common.Sep)
	if len(mm) >= 2 {
		//msg:  key-enode:C1:X1:X2....:Xn
		//msg:  key-enode1:NoReciv:enode2:C1
		DisMsg(res)
		return true
	}

	msgmap := make(map[string]string)
	err := json.Unmarshal([]byte(res), &msgmap)
	if err == nil {

	    //presign
	    if msgmap["Type"] == "PreSign" {
		ps := &PreSign{}
		if err = ps.UnmarshalJSON([]byte(msgmap["PreSign"]));err == nil {
		    w := workers[workid]
		    w.sid = ps.Nonce 
		    w.groupid = ps.Gid
		    w.DcrmFrom = ps.Pub
		    gcnt, _ := GetGroup(w.groupid)
		    w.NodeCnt = gcnt //TODO
		    w.ThresHold = gcnt
		    common.Debug("============================PreSign at RecvMsg.Run,get presign data==========================","pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index,"nonce",ps.Nonce)

		    dcrmpks, _ := hex.DecodeString(ps.Pub)
		    exsit,da := GetValueFromDb(string(dcrmpks[:]))
		    if !exsit {
			//common.Debug("============================PreSign at RecvMsg.Run,not exist presign data===========================","pubkey",ps.Pub)
			res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get presign data from db fail", Err: fmt.Errorf("get presign data from db fail")}
			ch <- res
			return false
		    }

		    pd,ok := da.(*PubKeyData)
		    if !ok {
			//common.Debug("============================PreSign at RecvMsg.Run,presign data error==========================","pubkey",ps.Pub)
			res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get presign data from db fail", Err: fmt.Errorf("get presign data from db fail")}
			ch <- res
			return false
		    }

		    save := (da.(*PubKeyData)).Save
		    ///sku1
		    da2 := GetSkU1FromLocalDb(string(dcrmpks[:]))
		    if da2 == nil {
			    res := RpcDcrmRes{Ret: "", Tip: "presign get sku1 fail", Err: fmt.Errorf("presign get sku1 fail")}
			    ch <- res
			    return false
		    }
		    sku1 := new(big.Int).SetBytes(da2)
		    if sku1 == nil {
			    res := RpcDcrmRes{Ret: "", Tip: "presign get sku1 fail", Err: fmt.Errorf("presign get sku1 fail")}
			    ch <- res
			    return false
		    }
		    //

		    exsit,da3 := GetValueFromDb(pd.Key)
		    ac,ok := da3.(*AcceptReqAddrData)
		    if ok {
			HandleC1Data(ac,w.sid,workid)
		    }

		    var ch1 = make(chan interface{}, 1)
		    pre := PreSign_ec3(w.sid,save,sku1,"ECDSA",ch1,workid)
		    if pre == nil {
			if syncpresign && !SynchronizePreSignData(w.sid,w.id,false) {
			    res := RpcDcrmRes{Ret: "", Tip: "presign fail", Err: fmt.Errorf("presign fail")}
			    ch <- res
			    return false
			}

			res := RpcDcrmRes{Ret: "", Tip: "presign fail", Err: fmt.Errorf("presign fail")}
			ch <- res
			return false
		    }

		    pre.Key = w.sid
		    pre.Gid = w.groupid
		    pre.Used = false
		    pre.Index = ps.Index

		    err = PutPreSignData(ps.Pub,ps.InputCode,ps.Gid,ps.Index,pre,true)
		    if err != nil {
			if syncpresign && !SynchronizePreSignData(w.sid,w.id,false) {
			    common.Info("================================PreSign at RecvMsg.Run, put pre-sign data to local db fail=====================","pick key",pre.Key,"pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index,"err",err)
			    res := RpcDcrmRes{Ret: "", Tip: "presign fail", Err: fmt.Errorf("presign fail")}
			    ch <- res
			    return false
			}

			common.Info("================================PreSign at RecvMsg.Run, put pre-sign data to local db fail=====================","pick key",pre.Key,"pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index,"err",err)
			res := RpcDcrmRes{Ret: "", Tip: "presign fail", Err: fmt.Errorf("presign fail")}
			ch <- res
			return false
		    }

		    if syncpresign && !SynchronizePreSignData(w.sid,w.id,true) {
			err = DeletePreSignData(ps.Pub,ps.InputCode,ps.Gid,pre.Key)
			if err == nil {
			    common.Debug("================================PreSign at RecvMsg.Run, delete pre-sign data from local db success=====================","pick key",pre.Key,"pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index)
			} else {
			    //.........
			    common.Info("================================PreSign at RecvMsg.Run, delete pre-sign data from local db fail=====================","pick key",pre.Key,"pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index,"err",err)
			}
			
			res := RpcDcrmRes{Ret: "", Tip: "presign fail", Err: fmt.Errorf("presign fail")}
			ch <- res
			return false
		    }

		    common.Debug("================================PreSign at RecvMsg.Run, put pre-sign data to local db success=====================","pick key",pre.Key,"pubkey",ps.Pub,"gid",ps.Gid,"index",ps.Index)
		    res := RpcDcrmRes{Ret: "success", Tip: "", Err: nil}
		    ch <- res
		    return true
		}
	    }

	    //signdata
	    if msgmap["Type"] == "SignData" {
		sd := &SignData{}
		if err = sd.UnmarshalJSON([]byte(msgmap["SignData"]));err == nil {

		    common.Debug("===============RecvMsg.Run,it is signdata===================","msgprex",sd.MsgPrex,"key",sd.Key)

		    ys := secp256k1.S256().Marshal(sd.Pkx, sd.Pky)
		    pubkeyhex := hex.EncodeToString(ys)

		    w := workers[workid]
		    w.sid = sd.Key
		    w.groupid = sd.GroupId
		    
		    w.NodeCnt = sd.NodeCnt
		    w.ThresHold = sd.ThresHold
		    
		    w.DcrmFrom = sd.DcrmFrom

		    dcrmpks, _ := hex.DecodeString(pubkeyhex)
		    exsit,da := GetValueFromDb(string(dcrmpks[:]))
		    if exsit {
			    pd,ok := da.(*PubKeyData)
			    if ok {
				exsit,da2 := GetValueFromDb(pd.Key)
				if exsit {
					ac,ok := da2.(*AcceptReqAddrData)
					if ok {
					    HandleC1Data(ac,sd.Key,workid)
					}
				}

			    }
		    }

		    var ch1 = make(chan interface{}, 1)
		    for i:=0;i < recalc_times;i++ {
			//common.Debug("===============RecvMsg.Run,sign recalc===================","i",i,"msgprex",sd.MsgPrex,"key",sd.Key)
			if len(ch1) != 0 {
			    <-ch1
			}

			//w.Clear2()
			//Sign_ec2(sd.Key, sd.Save, sd.Sku1, sd.Txhash, sd.Keytype, sd.Pkx, sd.Pky, ch1, workid)
			Sign_ec3(sd.Key,sd.Txhash,sd.Keytype,sd.Pkx,sd.Pky,ch1,workid,sd.Pre)
			common.Info("===============RecvMsg.Run, ec3 sign finish ===================","WaitMsgTimeGG20",WaitMsgTimeGG20)
			ret, _, cherr := GetChannelValue(WaitMsgTimeGG20 + 10, ch1)
			if ret != "" && cherr == nil {

			    ww, err2 := FindWorker(sd.MsgPrex)
			    if err2 != nil || ww == nil {
				res2 := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:no find worker", Err: fmt.Errorf("no find worker")}
				ch <- res2
				return false
			    }

			    common.Info("===============RecvMsg.Run, ec3 sign success ===================","i",i,"get ret",ret,"cherr",cherr,"msgprex",sd.MsgPrex,"key",sd.Key)

			    ww.rsv.PushBack(ret)
			    res2 := RpcDcrmRes{Ret: ret, Tip: "", Err: nil}
			    ch <- res2
			    return true 
			}
			
			common.Info("===============RecvMsg.Run,ec3 sign fail===================","ret",ret,"cherr",cherr,"msgprex",sd.MsgPrex,"key",sd.Key)
			//time.Sleep(time.Duration(3) * time.Second) //1000 == 1s
		    }	
		    
		    res2 := RpcDcrmRes{Ret: "", Tip: "sign fail", Err: fmt.Errorf("sign fail")}
		    ch <- res2
		    return false
		}/////
	    }

	    if msgmap["Type"] == "ComSignBrocastData" {
		signbrocast,err := UnCompressSignBrocastData(msgmap["ComSignBrocastData"])
		if err == nil {
		    common.Debug("===================================RecvMsg.Run,get sign cmd data and uncompress sign brocast data success=========================","get msg",signbrocast.Raw)
		    _,_,_,txdata,err := CheckRaw(signbrocast.Raw)
		    if err == nil {
			sig,ok := txdata.(*TxDataSign)
			if ok {
			    pickdata := make([]*PickHashData,0)
			    for _,vv := range signbrocast.PickHash {
				pre := GetPreSignData(sig.PubKey,sig.InputCode,sig.GroupId,vv.PickKey)
				if pre == nil {
				    common.Error("=====================================RecvMsg.Run,get pre-sign data fail======================","pubkey",sig.PubKey,"groupid",sig.GroupId,"pick key",vv.PickKey)
				    res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get pre-sign data fail", Err: fmt.Errorf("get pre-sign data fail.")}
				    ch <- res
				    return false
				}

				pd := &PickHashData{Hash:vv.Hash,Pre:pre}
				pickdata = append(pickdata,pd)
				DeletePreSignData(sig.PubKey,sig.InputCode,sig.GroupId,vv.PickKey)
			    }

			    signpick := &SignPickData{Raw:signbrocast.Raw,PickData:pickdata}
			    errtmp := DoSign(signpick,workid,self.sender,ch)
			    if errtmp == nil {
				return true
			    }

			    return false
			}
		    } else {
			common.Error("=================================RecvMsg.Run,get sign cmd raw data and check fail============================","signbrocast.Raw",signbrocast.Raw,"raw",res,"err",err)
		    }
		} else {
			common.Error("=================================RecvMsg.Run,get sign cmd raw data and uncompress sign brocast data fail============================","signbrocast.Raw",signbrocast.Raw,"raw",res,"err",err)
		}
	    }

	    if msgmap["Type"] == "ComSignData" {
		signpick,err := UnCompressSignData(msgmap["ComSignData"])
		if err == nil {
		    errtmp := DoSign(signpick,workid,self.sender,ch)
		    if errtmp == nil {
			return true
		    }
     
		    return false
		}
	    }
	}

	errtmp := DoReq(res,workid,self.sender,ch)
	if errtmp == nil {
	    return true
	}
	//common.Debug("================RecvMsg.Run, Unsupported raw data type.=================","raw",res,"err",errtmp)

	return false 
}

func HandleC1Data(ac *AcceptReqAddrData,key string,workid int) {
    //reshare only
    if ac == nil {
	exsit,da := GetReShareInfoData([]byte(key))
	if !exsit {
	    return
	}

	ac,ok := da.(*AcceptReShareData)
	if !ok || ac == nil {
	    return
	}

	_, enodes := GetGroup(ac.GroupId)
	nodes := strings.Split(enodes, common.Sep2)
	for _, node := range nodes {
	    node2 := ParseNode(node)
	    pk := "04" + node2 
	     h := coins.NewCryptocoinHandler("FSN")
	     if h == nil {
		continue
	     }

	    fr, err := h.PublicKeyToAddress(pk)
	    if err != nil {
		continue
	    }

	    c1data := key + "-" + fr
	    c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
	    if exist {
		DisAcceptMsg(c1.(string),workid)
		go C1Data.DeleteMap(strings.ToLower(c1data))
	    }
	}

	return
    }
    //reshare only

    if key == "" || workid < 0 || workid >= len(workers) {
	return
    }
   
    /////////bug
	_, enodes := GetGroup(ac.GroupId)
	nodes := strings.Split(enodes, common.Sep2)
	for _, node := range nodes {
	    node2 := ParseNode(node)
		c1data := key + "-" + node2 + common.Sep + "SS1"
		c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
		if exist {
		common.Info("===============HandleC1Data,exsit c1data in C1Data map for ss1====================","c1data",c1data)
		    DisMsg(c1.(string))
		    go C1Data.DeleteMap(strings.ToLower(c1data))
		}
    }
	for _, node := range nodes {
	    node2 := ParseNode(node)
		c1data := key + "-" + node2 + common.Sep + "C11" 
		c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
		if exist {
		    DisMsg(c1.(string))
		    go C1Data.DeleteMap(strings.ToLower(c1data))
		}
    }
	for _, node := range nodes {
	    node2 := ParseNode(node)
		c1data := key + "-" + node2 + common.Sep + "CommitBigVAB" 
		c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
		if exist {
		    DisMsg(c1.(string))
		    go C1Data.DeleteMap(strings.ToLower(c1data))
		}
    }
	for _, node := range nodes {
	    node2 := ParseNode(node)
		c1data := key + "-" + node2 + common.Sep + "C1" 
		c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
		if exist {
		    DisMsg(c1.(string))
		    go C1Data.DeleteMap(strings.ToLower(c1data))
		}
    }
    ////////////
    
    mms := strings.Split(ac.Sigs, common.Sep)
    if len(mms) < 3 { //1:eid1:acc1
	return
    }

    count := (len(mms)-1)/2
    for j := 0;j<count;j++ {
	from := mms[2*j+2]
	c1data := key + "-" + from
	c1, exist := C1Data.ReadMap(strings.ToLower(c1data))
	if exist {
	    DisAcceptMsg(c1.(string),workid)
	    go C1Data.DeleteMap(strings.ToLower(c1data))
	}
    }
}

func GetReqAddrRawValue(raw string) (string,string,string) {
    if raw == "" {
	return "","",""
    }

    tx := new(types.Transaction)
    raws := common.FromHex(raw)
    if err := rlp.DecodeBytes(raws, tx); err != nil {
	return "","",""
    }

    signer := types.NewEIP155Signer(big.NewInt(30400))
    from, err := types.Sender(signer,tx)
    if err != nil {
	return "","",""
    }

    var txtype string
    var timestamp string
    
    req := TxDataReqAddr{}
    err = json.Unmarshal(tx.Data(), &req)
    if err == nil && req.TxType == "REQDCRMADDR" {
	txtype = "REQDCRMADDR"
	timestamp = req.TimeStamp
    } else {
	acceptreq := TxDataAcceptReqAddr{}
	err = json.Unmarshal(tx.Data(), &acceptreq)
	if err == nil && acceptreq.TxType == "ACCEPTREQADDR" {
	    txtype = "ACCEPTREQADDR"
	    timestamp = acceptreq.TimeStamp
	} 
    }

    return from.Hex(),txtype,timestamp
}

func CheckReqAddrDulpRawReply(raw string,l *list.List) bool {
    if l == nil || raw == "" {
	return false
    }
   
    from,txtype,timestamp := GetReqAddrRawValue(raw)

    if from == "" || txtype == "" || timestamp == "" {
	return false
    }
    
    var next *list.Element
    for e := l.Front(); e != nil; e = next {
	next = e.Next()

	if e.Value == nil {
		continue
	}

	s := e.Value.(string)

	if s == "" {
		continue
	}

	if strings.EqualFold(raw,s) {
	   return false 
	}
	
	from2,txtype2,timestamp2 := GetReqAddrRawValue(s)
	if strings.EqualFold(from,from2) && strings.EqualFold(txtype,txtype2) {
	    t1,_ := new(big.Int).SetString(timestamp,10)
	    t2,_ := new(big.Int).SetString(timestamp2,10)
	    if t1.Cmp(t2) > 0 {
		l.Remove(e)
	    } else {
		return false
	    }
	}
    }

    return true
}

func GetReshareRawValue(raw string) (string,string,string) {
    if raw == "" {
	return "","",""
    }

    tx := new(types.Transaction)
    raws := common.FromHex(raw)
    if err := rlp.DecodeBytes(raws, tx); err != nil {
	return "","",""
    }

    signer := types.NewEIP155Signer(big.NewInt(30400))
    from, err := types.Sender(signer,tx)
    if err != nil {
	return "","",""
    }

    var txtype string
    var timestamp string
    
    rh := TxDataReShare{}
    err = json.Unmarshal(tx.Data(), &rh)
    if err == nil && rh.TxType == "RESHARE" {
	txtype = "RESHARE"
	timestamp = rh.TimeStamp
    } else {
	acceptrh := TxDataAcceptReShare{}
	err = json.Unmarshal(tx.Data(), &acceptrh)
	if err == nil && acceptrh.TxType == "ACCEPTRESHARE" {
	    txtype = "ACCEPTRESHARE"
	    timestamp = acceptrh.TimeStamp
	} 
    }

    return from.Hex(),txtype,timestamp
}

func CheckReshareDulpRawReply(raw string,l *list.List) bool {
    if l == nil || raw == "" {
	return false
    }
   
    from,txtype,timestamp := GetReshareRawValue(raw)

    if from == "" || txtype == "" || timestamp == "" {
	return false
    }
    
    var next *list.Element
    for e := l.Front(); e != nil; e = next {
	next = e.Next()

	if e.Value == nil {
		continue
	}

	s := e.Value.(string)

	if s == "" {
		continue
	}

	if strings.EqualFold(raw,s) {
	    return false 
	}
	
	from2,txtype2,timestamp2 := GetReshareRawValue(s)
	if strings.EqualFold(from,from2) && strings.EqualFold(txtype,txtype2) {
	    t1,_ := new(big.Int).SetString(timestamp,10)
	    t2,_ := new(big.Int).SetString(timestamp2,10)
	    if t1.Cmp(t2) > 0 {
		l.Remove(e)
	    } else {
		return false
	    }
	}
    }

    return true
}

func GetSignRawValue(raw string) (string,string,string) {
    if raw == "" {
	return "","",""
    }

    tx := new(types.Transaction)
    raws := common.FromHex(raw)
    if err := rlp.DecodeBytes(raws, tx); err != nil {
	return "","",""
    }

    signer := types.NewEIP155Signer(big.NewInt(30400))
    from, err := types.Sender(signer,tx)
    if err != nil {
	return "","",""
    }

    var txtype string
    var timestamp string
    
    sig := TxDataSign{}
    err = json.Unmarshal(tx.Data(), &sig)
    if err == nil && sig.TxType == "SIGN" {
	txtype = "SIGN"
	timestamp = sig.TimeStamp
    } else {
	pre := TxDataPreSignData{}
	err = json.Unmarshal(tx.Data(), &pre)
	if err == nil && pre.TxType == "PRESIGNDATA" {
	    txtype = "PRESIGNDATA"
	    //timestamp = pre.TimeStamp
	} else {
	    acceptsig := TxDataAcceptSign{}
	    err = json.Unmarshal(tx.Data(), &acceptsig)
	    if err == nil && acceptsig.TxType == "ACCEPTSIGN" {
		txtype = "ACCEPTSIGN"
		timestamp = acceptsig.TimeStamp
	    }
	}
    }

    return from.Hex(),txtype,timestamp
}

func CheckSignDulpRawReply(raw string,l *list.List) bool {
    if l == nil || raw == "" {
	return false
    }
   
    from,txtype,timestamp := GetSignRawValue(raw)

    if from == "" || txtype == "" || timestamp == "" {
	return false
    }
    
    var next *list.Element
    for e := l.Front(); e != nil; e = next {
	next = e.Next()

	if e.Value == nil {
		continue
	}

	s := e.Value.(string)

	if s == "" {
		continue
	}

	if strings.EqualFold(raw,s) {
	   return false 
	}
	
	from2,txtype2,timestamp2 := GetSignRawValue(s)
	if strings.EqualFold(from,from2) && strings.EqualFold(txtype,txtype2) {
	    t1,_ := new(big.Int).SetString(timestamp,10)
	    t2,_ := new(big.Int).SetString(timestamp2,10)
	    if t1.Cmp(t2) > 0 {
		l.Remove(e)
	    } else {
		return false
	    }
	}
    }

    return true
}

func DisAcceptMsg(raw string,workid int) {
    defer func() {
        if r := recover(); r != nil {
	    fmt.Errorf("DisAcceptMsg Runtime error: %v\n%v", r, string(debug.Stack()))
	    return
        }
    }()

    if raw == "" || workid < 0 || workid >= len(workers) {
	return
    }

    w := workers[workid]
    if w == nil {
	return
    }

    key,from,_,txdata,err := CheckRaw(raw)
    //common.Debug("=====================DisAcceptMsg=================","key",key,"err",err)
    if err != nil {
	return
    }
    
    _,ok := txdata.(*TxDataReqAddr)
    if ok {
	if Find(w.msg_acceptreqaddrres,raw) {
		return
	}

	if !CheckReqAddrDulpRawReply(raw,w.msg_acceptreqaddrres) {
	    return
	}

	w.msg_acceptreqaddrres.PushBack(raw)
	if w.msg_acceptreqaddrres.Len() >= w.NodeCnt {
	    if !CheckReply(w.msg_acceptreqaddrres,Rpc_REQADDR,key) {
		return
	    }

	    w.bacceptreqaddrres <- true
	    exsit,da := GetReqAddrInfoData([]byte(key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptReqAddrData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptReqAddrChan <- "go on"
	}
    }
    
    sig2,ok := txdata.(*TxDataSign)
    if ok {
	    //common.Debug("======================DisAcceptMsg, get the msg and it is sign tx===========================","key",key,"from",from,"raw",raw)
	if Find(w.msg_acceptsignres, raw) {
	    common.Info("======================DisAcceptMsg,the msg is sign tx,and already in list.===========================","key",key,"from",from)
	    return
	}

	if !CheckSignDulpRawReply(raw,w.msg_acceptsignres) {
	    return
	}

	    common.Debug("======================DisAcceptMsg,the msg is sign tx,and put it into list.===========================","key",key,"from",from,"sig",sig2)
	w.msg_acceptsignres.PushBack(raw)
	if w.msg_acceptsignres.Len() >= w.ThresHold {
	    if !CheckReply(w.msg_acceptsignres,Rpc_SIGN,key) {
		//common.Info("=====================DisAcceptMsg,check reply fail===================","key",key,"from",from)
		return
	    }

	    //common.Info("=====================DisAcceptMsg,check reply success and will set timeout channel===================","key",key,"from",from)
	    w.bacceptsignres <- true
	    exsit,da := GetSignInfoData([]byte(key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptSignData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptSignChan <- "go on"
	}
    }
    
    _,ok = txdata.(*TxDataReShare)
    if ok {
	if Find(w.msg_acceptreshareres, raw) {
	    return
	}

	if !CheckReshareDulpRawReply(raw,w.msg_acceptreshareres) {
	    return
	}

	w.msg_acceptreshareres.PushBack(raw)
	if w.msg_acceptreshareres.Len() >= w.NodeCnt {
	    if !CheckReply(w.msg_acceptreshareres,Rpc_RESHARE,key) {
		return
	    }

	    w.bacceptreshareres <- true
	    exsit,da := GetReShareInfoData([]byte(key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptReShareData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptReShareChan <- "go on"
	}
    }
    
    acceptreq,ok := txdata.(*TxDataAcceptReqAddr)
    if ok {
	if Find(w.msg_acceptreqaddrres,raw) {
		return
	}

	if !CheckReqAddrDulpRawReply(raw,w.msg_acceptreqaddrres) {
	    return
	}

	w.msg_acceptreqaddrres.PushBack(raw)
	if w.msg_acceptreqaddrres.Len() >= w.NodeCnt {
	    if !CheckReply(w.msg_acceptreqaddrres,Rpc_REQADDR,acceptreq.Key) {
		return
	    }

	    w.bacceptreqaddrres <- true
	    exsit,da := GetReqAddrInfoData([]byte(acceptreq.Key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptReqAddrData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptReqAddrChan <- "go on"
	}
    }
    
    acceptsig,ok := txdata.(*TxDataAcceptSign)
    if ok {
	    common.Debug("======================DisAcceptMsg, get the msg and it is accept sign tx===========================","key",acceptsig.Key,"from",from,"raw",raw)
	if Find(w.msg_acceptsignres, raw) {
	    common.Info("======================DisAcceptMsg,the msg is accept sign tx,and already in list.===========================","sig key",acceptsig.Key,"from",from)
	    return
	}

	if !CheckSignDulpRawReply(raw,w.msg_acceptsignres) {
	    return
	}

	//common.Debug("======================DisAcceptMsg,the msg is accept sign tx,and put it into list.===========================","sig key",acceptsig.Key,"from",from,"accept sig",acceptsig)
	w.msg_acceptsignres.PushBack(raw)
	if w.msg_acceptsignres.Len() >= w.ThresHold {
	    if !CheckReply(w.msg_acceptsignres,Rpc_SIGN,acceptsig.Key) {
		    //common.Info("======================DisAcceptMsg,the msg is accept sign tx,and check reply fail.===========================","sig key",acceptsig.Key,"from",from)
		return
	    }

	    common.Debug("======================DisAcceptMsg,the msg is accept sign tx,and check reply success and will set timeout channel.===========================","sig key",acceptsig.Key,"from",from)
	    w.bacceptsignres <- true
	    exsit,da := GetSignInfoData([]byte(acceptsig.Key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptSignData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptSignChan <- "go on"
	}
    }
    
    acceptreshare,ok := txdata.(*TxDataAcceptReShare)
    if ok {
	if Find(w.msg_acceptreshareres, raw) {
	    return
	}

	if !CheckReshareDulpRawReply(raw,w.msg_acceptreshareres) {
	    return
	}

	w.msg_acceptreshareres.PushBack(raw)
	if w.msg_acceptreshareres.Len() >= w.NodeCnt {
	    if !CheckReply(w.msg_acceptreshareres,Rpc_RESHARE,acceptreshare.Key) {
		return
	    }

	    w.bacceptreshareres <- true
	    exsit,da := GetReShareInfoData([]byte(acceptreshare.Key))
	    if !exsit {
		return
	    }

	    ac,ok := da.(*AcceptReShareData)
	    if !ok || ac == nil {
		return
	    }

	    workers[ac.WorkId].acceptReShareChan <- "go on"
	}
    }
}

func DoReq(raw string,workid int,sender string,ch chan interface{}) error {
    if raw == "" || workid < 0 || sender == "" {
	res := RpcDcrmRes{Ret: "", Tip: "param error", Err: fmt.Errorf("param error")}
	ch <- res
	return fmt.Errorf("param error")
    }

    key,from,nonce,txdata,err := CheckRaw(raw)
    if err != nil {
	common.Error("===============DoReq,get raw data and check error===================","err ",err)
	res := RpcDcrmRes{Ret: "", Tip: err.Error(), Err: err}
	ch <- res
	return err
    }
    
    req,ok := txdata.(*TxDataReqAddr)
    if ok {
	common.Info("===============DoReq, get reqaddr cmd data==================","raw data",raw,"key ",key,"from ",from,"nonce ",nonce,"txdata ",req)
	exsit,_ := GetReqAddrInfoData([]byte(key))
	if !exsit {
	    cur_nonce, _, _ := GetReqAddrNonce(from)
	    cur_nonce_num, _ := new(big.Int).SetString(cur_nonce, 10)
	    new_nonce_num, _ := new(big.Int).SetString(nonce, 10)
	    //common.Debug("===============DoReq============","reqaddr cur_nonce_num ",cur_nonce_num,"reqaddr new_nonce_num ",new_nonce_num,"key ",key)
	    if new_nonce_num.Cmp(cur_nonce_num) >= 0 {
		_, err := SetReqAddrNonce(from,nonce)
		if err == nil {
		    ars := GetAllReplyFromGroup(workid,req.GroupId,Rpc_REQADDR,sender)
		    sigs,err := GetGroupSigsDataByRaw(raw) 
		    //common.Debug("=================DoReq================","get group sigs ",sigs,"err ",err,"key ",key)
		    if err != nil {
			res := RpcDcrmRes{Ret: "", Tip: err.Error(), Err: err}
			ch <- res
			return err
		    }

		    ac := &AcceptReqAddrData{Initiator:sender,Account: from, Cointype: "ALL", GroupId: req.GroupId, Nonce: nonce, LimitNum: req.ThresHold, Mode: req.Mode, TimeStamp: req.TimeStamp, Deal: "false", Accept: "false", Status: "Pending", PubKey: "", Tip: "", Error: "", AllReply: ars, WorkId: workid,Sigs:sigs}
		    err = SaveAcceptReqAddrData(ac)
		    common.Info("===================DoReq, call SaveAcceptReqAddrData finish====================","from",from,"key ",key)
		   if err == nil {
			rch := make(chan interface{}, 1)
			w := workers[workid]
			w.sid = key 
			w.groupid = req.GroupId
			w.limitnum = req.ThresHold
			gcnt, _ := GetGroup(w.groupid)
			w.NodeCnt = gcnt
			w.ThresHold = w.NodeCnt

			nums := strings.Split(w.limitnum, "/")
			if len(nums) == 2 {
			    nodecnt, err := strconv.Atoi(nums[1])
			    if err == nil {
				w.NodeCnt = nodecnt
			    }

			    th, err := strconv.Atoi(nums[0])
			    if err == nil {
				w.ThresHold = th 
			    }
			}

			if req.Mode == "0" { // self-group
				////
				var reply bool
				var tip string
				timeout := make(chan bool, 1)
				go func(wid int) {
					cur_enode = discover.GetLocalID().String() //GetSelfEnode()
					agreeWaitTime := 2 * time.Minute
					agreeWaitTimeOut := time.NewTicker(agreeWaitTime)
					if wid < 0 || wid >= len(workers) || workers[wid] == nil {
						ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)	
						_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId, nonce, req.ThresHold, req.Mode, "false", "false", "Failure", "", "workid error", "workid error", ars, wid,"")
						if err != nil {
						    tip = "accept reqaddr error"
						    reply = false
						    timeout <- true
						    return
						}

						tip = "worker id error"
						reply = false
						timeout <- true
						return
					}

					wtmp2 := workers[wid]
					for {
						select {
						case account := <-wtmp2.acceptReqAddrChan:
							common.Debug("(self *RecvMsg) Run(),", "account= ", account, "key = ", key)
							ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)
							common.Info("================== DoReq,get all AcceptReqAddrRes====================","result ",ars,"key ",key)
							
							//bug
							reply = true
							for _,nr := range ars {
							    if !strings.EqualFold(nr.Status,"Agree") {
								reply = false
								break
							    }
							}

							if !reply {
								tip = "don't accept req addr"
								_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId,nonce, req.ThresHold, req.Mode, "false", "false", "Failure", "", "don't accept req addr", "don't accept req addr", ars, wid,"")
								if err != nil {
								    tip = "don't accept req addr and accept reqaddr error"
								    timeout <- true
								    return
								}
							} else {
								tip = ""
								_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId,nonce, req.ThresHold, req.Mode, "false", "true", "Pending", "", "", "", ars, wid,"")
								if err != nil {
								    tip = "accept reqaddr error"
								    timeout <- true
								    return
								}
							}

							timeout <- true
							return
						case <-agreeWaitTimeOut.C:
							common.Info("================== DoReq, agree wait timeout==================","key ",key)
							ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)
							//bug: if self not accept and timeout
							_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId, nonce, req.ThresHold, req.Mode, "false", "false", "Timeout", "", "get other node accept req addr result timeout", "get other node accept req addr result timeout", ars, wid,"")
							if err != nil {
							    tip = "get other node accept req addr result timeout and accept reqaddr fail"
							    reply = false
							    timeout <- true
							    return
							}

							tip = "get other node accept req addr result timeout"
							reply = false

							timeout <- true
							return
						}
					}
				}(workid)

				if len(workers[workid].acceptWaitReqAddrChan) == 0 {
					workers[workid].acceptWaitReqAddrChan <- "go on"
				}

				DisAcceptMsg(raw,workid)
				HandleC1Data(ac,key,workid)

				<-timeout

				//common.Debug("================== DoReq======================","the terminal accept req addr result ",reply,"key ",key)

				ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)
				if !reply {
					if tip == "get other node accept req addr result timeout" {
						_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId, nonce, req.ThresHold, req.Mode, "false", "", "Timeout", "", tip, "don't accept req addr.", ars, workid,"")
					} else {
						_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId, nonce, req.ThresHold, req.Mode, "false", "", "Failure", "", tip, "don't accept req addr.", ars, workid,"")
					}

					if err != nil {
					    res := RpcDcrmRes{Ret:"", Tip: tip, Err: fmt.Errorf("don't accept req addr.")}
					    ch <- res
					    return fmt.Errorf("don't accept req addr.")
					}

					res := RpcDcrmRes{Ret: strconv.Itoa(workid) + common.Sep + "rpc_req_dcrmaddr", Tip: tip, Err: fmt.Errorf("don't accept req addr.")}
					ch <- res
					return fmt.Errorf("don't accept req addr.")
				}
			} else {
				if len(workers[workid].acceptWaitReqAddrChan) == 0 {
					workers[workid].acceptWaitReqAddrChan <- "go on"
				}

				ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)
				_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId,nonce, req.ThresHold, req.Mode, "false", "true", "Pending", "", "", "", ars, workid,"")
				if err != nil {
				    res := RpcDcrmRes{Ret:"", Tip: err.Error(), Err: err}
				    ch <- res
				    return err
				}
			}

			dcrm_genPubKey(w.sid, from, "ALL", rch, req.Mode, nonce)
			chret, tip, cherr := GetChannelValue(waitall, rch)
			common.Info("================== DoReq, req addr finish ===================","get pubkey",chret,"key",key,"err",cherr)
			if cherr != nil {
				ars := GetAllReplyFromGroup(w.id,req.GroupId,Rpc_REQADDR,sender)
				_,err = AcceptReqAddr(sender,from, "ALL", req.GroupId, nonce, req.ThresHold, req.Mode, "false", "", "Failure", "", tip, cherr.Error(), ars, workid,"")
				if err != nil {
				    res := RpcDcrmRes{Ret:"", Tip:err.Error(), Err:err}
				    ch <- res
				    return err
				}

				res := RpcDcrmRes{Ret: strconv.Itoa(workid) + common.Sep + "rpc_req_dcrmaddr", Tip: tip, Err: cherr}
				ch <- res
				return cherr 
			}

			res := RpcDcrmRes{Ret: strconv.Itoa(workid) + common.Sep + "rpc_req_dcrmaddr" + common.Sep + chret, Tip: "", Err: nil}
			ch <- res
			return nil
		   }
		}
	    }
	}
    }

    rh,ok := txdata.(*TxDataReShare)
    if ok {
	ars := GetAllReplyFromGroup(workid,rh.GroupId,Rpc_RESHARE,sender)
	sigs,err := GetGroupSigsDataByRaw(raw) 
	common.Debug("=================DoReq,get reshare cmd data=================","raw",raw,"get group sigs ",sigs,"key ",key,"err",err)
	if err != nil {
	    res := RpcDcrmRes{Ret: "", Tip: err.Error(), Err: err}
	    ch <- res
	    return err
	}

	ac := &AcceptReShareData{Initiator:sender,Account: from, GroupId: rh.GroupId, TSGroupId:rh.TSGroupId, PubKey: rh.PubKey, LimitNum: rh.ThresHold, PubAccount:rh.Account, Mode:rh.Mode, Sigs:sigs, TimeStamp: rh.TimeStamp, Deal: "false", Accept: "false", Status: "Pending", NewSk: "", Tip: "", Error: "", AllReply: ars, WorkId:workid}
	err = SaveAcceptReShareData(ac)
	common.Info("===================DoReq,finish call SaveAcceptReShareData======================","err ",err,"workid ",workid,"account ",from,"group id ",rh.GroupId,"pubkey ",rh.PubKey,"threshold ",rh.ThresHold,"key ",key)
	if err == nil {
	    w := workers[workid]
	    w.sid = key 
	    w.groupid = rh.TSGroupId 
	    w.limitnum = rh.ThresHold
	    gcnt, _ := GetGroup(w.groupid)
	    w.NodeCnt = gcnt
	    w.ThresHold = w.NodeCnt

	    nums := strings.Split(w.limitnum, "/")
	    if len(nums) == 2 {
		nodecnt, err := strconv.Atoi(nums[1])
		if err == nil {
		    w.NodeCnt = nodecnt
		}

		w.ThresHold = gcnt
	    }

	    w.DcrmFrom = rh.PubKey  // pubkey replace dcrmfrom in reshare 

	    var reply bool
	    var tip string
	    timeout := make(chan bool, 1)
	    go func(wid int) {
		    cur_enode = discover.GetLocalID().String() //GetSelfEnode()
		    agreeWaitTime := 10 * time.Minute
		    agreeWaitTimeOut := time.NewTicker(agreeWaitTime)

		    wtmp2 := workers[wid]

		    for {
			    select {
			    case account := <-wtmp2.acceptReShareChan:
				    common.Debug("(self *RecvMsg) Run(),", "account= ", account, "key = ", key)
				    ars := GetAllReplyFromGroup(w.id,rh.GroupId,Rpc_RESHARE,sender)
				    common.Info("================== DoReq, get all AcceptReShareRes================","result ",ars,"key ",key)
				    
				    //bug
				    reply = true
				    for _,nr := range ars {
					if !strings.EqualFold(nr.Status,"Agree") {
					    reply = false
					    break
					}
				    }
				    //

				    if !reply {
					    tip = "don't accept reshare"
					    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "false", "Failure", "", "don't accept reshare", "don't accept reshare", nil, wid)
				    } else {
					    tip = ""
					    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "false", "pending", "", "", "", ars, wid)
				    }

				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }

				    timeout <- true
				    return
			    case <-agreeWaitTimeOut.C:
				    common.Info("================== DoReq, agree wait timeout===================","raw ",raw,"key ",key)
				    ars := GetAllReplyFromGroup(w.id,rh.GroupId,Rpc_RESHARE,sender)
				    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "false", "Timeout", "", "get other node accept reshare result timeout", "get other node accept reshare result timeout", ars, wid)
				    reply = false
				    tip = "get other node accept reshare result timeout"
				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }
				    //

				    timeout <- true
				    return
			    }
		    }
	    }(workid)

	    if len(workers[workid].acceptWaitReShareChan) == 0 {
		    workers[workid].acceptWaitReShareChan <- "go on"
	    }

	    DisAcceptMsg(raw,workid)
	    HandleC1Data(nil,key,workid)
	    
	    <-timeout

	    if !reply {
		    //////////////////////reshare result start/////////////////////////
		    if tip == "get other node accept reshare result timeout" {
			    ars := GetAllReplyFromGroup(workid,rh.GroupId,Rpc_RESHARE,sender)
			    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold, rh.Mode,"false", "", "Timeout", "", "get other node accept reshare result timeout", "get other node accept reshare result timeout", ars,workid)
		    } else {
			    /////////////TODO tmp
			    //sid-enode:SendReShareRes:Success:rsv
			    //sid-enode:SendReShareRes:Fail:err
			    mp := []string{w.sid, cur_enode}
			    enode := strings.Join(mp, "-")
			    s0 := "SendReShareRes"
			    s1 := "Fail"
			    s2 := "don't accept reshare."
			    ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2
			    SendMsgToDcrmGroup(ss, rh.GroupId)
			    DisMsg(ss)
			    _, _, err := GetChannelValue(ch_t, w.bsendreshareres)
			    ars := GetAllReplyFromGroup(w.id,rh.GroupId,Rpc_RESHARE,sender)
			    if err != nil {
				    tip = "get other node terminal accept reshare result timeout" ////bug
				    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Timeout", "", tip,tip, ars, workid)
				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }

			    } else if w.msg_sendreshareres.Len() != w.ThresHold {
				    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold, rh.Mode,"false", "", "Failure", "", "get other node reshare result fail","get other node reshare result fail",ars, workid)
				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }
			    } else {
				    reply2 := "false"
				    lohash := ""
				    iter := w.msg_sendreshareres.Front()
				    for iter != nil {
					    mdss := iter.Value.(string)
					    ms := strings.Split(mdss, common.Sep)
					    if strings.EqualFold(ms[2], "Success") {
						    reply2 = "true"
						    lohash = ms[3]
						    break
					    }

					    lohash = ms[3]
					    iter = iter.Next()
				    }

				    if reply2 == "true" {
					    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold, rh.Mode,"true", "true", "Success", lohash," "," ",ars, workid)
					    if err != nil {
						tip = tip + " and accept reshare data fail"
					    }
				    } else {
					    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Failure", "",lohash,lohash,ars, workid)
					    if err != nil {
						tip = tip + " and accept reshare data fail"
					    }
				    }
			    }
		    }

		    res2 := RpcDcrmRes{Ret: "", Tip: tip, Err: fmt.Errorf("don't accept reshare.")}
		    ch <- res2
		    return fmt.Errorf("don't accept reshare.")
	    }

	    rch := make(chan interface{}, 1)
	    reshare(w.sid,from,rh.GroupId,rh.PubKey,rh.Account,rh.Mode,sigs,rch)
	    chret, tip, cherr := GetChannelValue(ch_t, rch)
	    if chret != "" {
		    res2 := RpcDcrmRes{Ret: chret, Tip: "", Err: nil}
		    ch <- res2
		    return nil 
	    }

	    if tip == "get other node accept reshare result timeout" {
		    ars := GetAllReplyFromGroup(workid,rh.GroupId,Rpc_RESHARE,sender)
		    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Timeout", "", "get other node accept reshare result timeout", "get other node accept reshare result timeout", ars,workid)
	    } else {
		    /////////////TODO tmp
		    //sid-enode:SendReShareRes:Success:rsv
		    //sid-enode:SendReShareRes:Fail:err
		    mp := []string{w.sid, cur_enode}
		    enode := strings.Join(mp, "-")
		    s0 := "SendReShareRes"
		    s1 := "Fail"
		    s2 := "don't accept reshare."
		    ss := enode + common.Sep + s0 + common.Sep + s1 + common.Sep + s2
		    SendMsgToDcrmGroup(ss, rh.GroupId)
		    DisMsg(ss)
		    _, _, err := GetChannelValue(ch_t, w.bsendreshareres)
		    ars := GetAllReplyFromGroup(w.id,rh.GroupId,Rpc_RESHARE,sender)
		    if err != nil {
			    tip = "get other node terminal accept reshare result timeout" ////bug
			    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Timeout", "", tip,tip, ars, workid)
			    if err != nil {
				tip = tip + " and accept reshare data fail"
			    }
		    } else if w.msg_sendsignres.Len() != w.ThresHold {
			    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Failure", "", "get other node reshare result fail","get other node reshare result fail",ars, workid)
			    if err != nil {
				tip = tip + " and accept reshare data fail"
			    }
		    } else {
			    reply2 := "false"
			    lohash := ""
			    iter := w.msg_sendreshareres.Front()
			    for iter != nil {
				    mdss := iter.Value.(string)
				    ms := strings.Split(mdss, common.Sep)
				    if strings.EqualFold(ms[2], "Success") {
					    reply2 = "true"
					    lohash = ms[3]
					    break
				    }

				    lohash = ms[3]
				    iter = iter.Next()
			    }

			    if reply2 == "true" {
				    _,err = AcceptReShare(sender,from, rh.GroupId, rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"true", "true", "Success", lohash," "," ",ars, workid)
				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }
			    } else {
				    _,err = AcceptReShare(sender,from, rh.GroupId,rh.TSGroupId,rh.PubKey, rh.ThresHold,rh.Mode,"false", "", "Failure", "",lohash,lohash,ars,workid)
				    if err != nil {
					tip = tip + " and accept reshare data fail"
				    }
			    }
		    }
	    }

	    if cherr != nil {
		    res2 := RpcDcrmRes{Ret:"", Tip: tip, Err: cherr}
		    ch <- res2
		    return cherr 
	    }

	    res2 := RpcDcrmRes{Ret:"", Tip: tip, Err: fmt.Errorf("reshare fail.")}
	    ch <- res2
	    return fmt.Errorf("reshare fail.")
	}
    }

    acceptreq,ok := txdata.(*TxDataAcceptReqAddr)
    if ok {
	common.Info("===============DoReq, get reqaddr accept data and check success======================","raw ",raw,"key ",acceptreq.Key,"from ",from,"txdata ",acceptreq)
	w, err := FindWorker(acceptreq.Key)
	if err != nil || w == nil {
	    c1data := acceptreq.Key + "-" + from
	    C1Data.WriteMap(strings.ToLower(c1data),raw)
	    res := RpcDcrmRes{Ret:"Failure", Tip: "worker was not found.", Err: fmt.Errorf("worker was not found.")}
	    ch <- res
	    return fmt.Errorf("worker was not found.")
	}

	exsit,da := GetReqAddrInfoData([]byte(acceptreq.Key))
	if !exsit {
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get reqaddr accept data fail from local db", Err: fmt.Errorf("get reqaddr accept data fail from local db")}
	    ch <- res
	    return fmt.Errorf("get reqaddr accept data fail from local db")
	}

	ac,ok := da.(*AcceptReqAddrData)
	if !ok || ac == nil {
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get reqaddr accept data error from local db", Err: fmt.Errorf("get reqaddr accept data error from local db")}
	    ch <- res
	    return fmt.Errorf("get reqaddr accept data error from local db")
	}

	status := "Pending"
	accept := "false"
	if acceptreq.Accept == "AGREE" {
		accept = "true"
	} else {
		status = "Failure"
	}

	id,_ := GetWorkerId(w)
	DisAcceptMsg(raw,id)
	HandleC1Data(ac,acceptreq.Key,id)

	ars := GetAllReplyFromGroup(id,ac.GroupId,Rpc_REQADDR,ac.Initiator)
	tip, err := AcceptReqAddr(ac.Initiator,ac.Account, ac.Cointype, ac.GroupId, ac.Nonce, ac.LimitNum, ac.Mode, "false", accept, status, "", "", "", ars, ac.WorkId,"")
	if err != nil {
	    res := RpcDcrmRes{Ret:"Failure", Tip: tip, Err: err}
	    ch <- res
	    return err 
	}

	res := RpcDcrmRes{Ret:"Success", Tip: "", Err: nil}
	ch <- res
	return nil
    }

    acceptsig,ok := txdata.(*TxDataAcceptSign)
    if ok {
	common.Info("============================DoReq, get sign accept data and check success=====================","key ",acceptsig.Key,"from ",from,"accept",acceptsig.Accept,"raw data",raw)
	w, err := FindWorker(acceptsig.Key)
	if err != nil || w == nil {
	    c1data := acceptsig.Key + "-" + from
	    C1Data.WriteMap(strings.ToLower(c1data),raw)
	    res := RpcDcrmRes{Ret:"Failure", Tip: "worker was not found.", Err: fmt.Errorf("worker was not found.")}
	    ch <- res
	    return fmt.Errorf("worker was not found.")
	}

	exsit,da := GetSignInfoData([]byte(acceptsig.Key))
	if !exsit {
	    common.Error("===============DoReq, get sign accept data fail from local db=====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get sign accept data fail from local db", Err: fmt.Errorf("get sign accept data fail from local db")}
	    ch <- res
	    return fmt.Errorf("get sign accept data fail from local db.")
	}

	ac,ok := da.(*AcceptSignData)
	if !ok || ac == nil {
		common.Error("===============DoReq, get sign accept data error from local db=====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get sign accept data error from local db", Err: fmt.Errorf("get sign accept data error from local db")}
	    ch <- res
	    return fmt.Errorf("get sign accept data error from local db")
	}

	if ac.Deal == "true" || ac.Status == "Success" || ac.Status == "Failure" || ac.Status == "Timeout" {
	    common.Error("===============DoReq, sign has handled before=====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret:"", Tip: "sign has handled before", Err: fmt.Errorf("sign has handled before")}
	    ch <- res
	    return fmt.Errorf("sign has handled before")
	}

	status := "Pending"
	accept := "false"
	if acceptsig.Accept == "AGREE" {
		accept = "true"
	} else {
		status = "Failure"
	}

	id,_ := GetWorkerId(w)
	DisAcceptMsg(raw,id)
	reqaddrkey := GetReqAddrKeyByOtherKey(acceptsig.Key,Rpc_SIGN)
	exsit,da = GetValueFromDb(reqaddrkey)
	if !exsit {
	    common.Error("===============DoReq, get reqaddr sigs data fail=====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get reqaddr sigs data fail", Err: fmt.Errorf("get reqaddr sigs data fail")}
	    ch <- res
	    return fmt.Errorf("get reqaddr sigs data fail") 
	}
	acceptreqdata,ok := da.(*AcceptReqAddrData)
	if !ok || acceptreqdata == nil {
		common.Error("===============DoReq, get reqaddr sigs data error. =====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret: "", Tip: "dcrm back-end internal error:get reqaddr sigs data fail", Err: fmt.Errorf("get reqaddr sigs data fail")}
	    ch <- res
	    return fmt.Errorf("get reqaddr sigs data fail") 
	}

	HandleC1Data(acceptreqdata,acceptsig.Key,id)

	ars := GetAllReplyFromGroup(id,ac.GroupId,Rpc_SIGN,ac.Initiator)
	if ac.Deal == "true" || ac.Status == "Success" || ac.Status == "Failure" || ac.Status == "Timeout" {
	    common.Info("===============DoReq, get sign accept data and sign has handled before =====================","key ",acceptsig.Key,"from ",from)
	    res := RpcDcrmRes{Ret:"", Tip: "sign has handled before", Err: fmt.Errorf("sign has handled before")}
	    ch <- res
	    return fmt.Errorf("sign has handled before")
	}

	//common.Debug("=======================DoReq,set sign status by sign accept data=============================","status",status,"key",acceptsig.Key)
	tip, err := AcceptSign(ac.Initiator,ac.Account, ac.PubKey, ac.MsgHash, ac.Keytype, ac.GroupId, ac.Nonce,ac.LimitNum,ac.Mode,"false", accept, status, "", "", "", ars, ac.WorkId)
	if err != nil {
	    res := RpcDcrmRes{Ret:"Failure", Tip: tip, Err: err}
	    ch <- res
	    return err 
	}

	res := RpcDcrmRes{Ret:"Success", Tip: "", Err: nil}
	ch <- res
	return nil
    }

    acceptrh,ok := txdata.(*TxDataAcceptReShare)
    if ok {
	common.Info("===============DoReq, get reshare accept raw data and check success=====================","raw data",raw,"key ",acceptrh.Key,"from ",from,"txdata ",acceptrh)
	w, err := FindWorker(acceptrh.Key)
	if err != nil || w == nil {
	    c1data := acceptrh.Key + "-" + from
	    C1Data.WriteMap(strings.ToLower(c1data),raw)
	    res := RpcDcrmRes{Ret:"Failure", Tip: "worker was not found.", Err: fmt.Errorf("worker was not found.")}
	    ch <- res
	    return fmt.Errorf("worker was not found.")
	}

	exsit,da := GetReShareInfoData([]byte(acceptrh.Key))
	if !exsit {
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get reshare accept data fail from local db", Err: fmt.Errorf("get reshare accept data fail from local db")}
	    ch <- res
	    return fmt.Errorf("get reshare accept data fail from local db.")
	}

	ac,ok := da.(*AcceptReShareData)
	if !ok || ac == nil {
	    res := RpcDcrmRes{Ret:"Failure", Tip: "dcrm back-end internal error:get reshare accept data error from local db", Err: fmt.Errorf("get reshare accept data error from local db")}
	    ch <- res
	    return fmt.Errorf("get reshare accept data error from local db")
	}

	status := "Pending"
	accept := "false"
	if acceptrh.Accept == "AGREE" {
		accept = "true"
	} else {
		status = "Failure"
	}

	id,_ := GetWorkerId(w)
	DisAcceptMsg(raw,id)
	HandleC1Data(nil,acceptrh.Key,id)

	ars := GetAllReplyFromGroup(id,ac.GroupId,Rpc_RESHARE,ac.Initiator)
	tip,err := AcceptReShare(ac.Initiator,ac.Account, ac.GroupId, ac.TSGroupId,ac.PubKey, ac.LimitNum, ac.Mode,"false", accept, status, "", "", "", ars,ac.WorkId)
	if err != nil {
	    res := RpcDcrmRes{Ret:"Failure", Tip: tip, Err: err}
	    ch <- res
	    return err 
	}

	res := RpcDcrmRes{Ret:"Success", Tip: "", Err: nil}
	ch <- res
	return nil
    }
	
    //common.Debug("===============DoReq, Unsupported raw data type and return fail ==================","key ",key,"from ",from,"nonce ",nonce)
    res := RpcDcrmRes{Ret: "", Tip: "Unsupported raw data type.", Err: fmt.Errorf("Unsupported raw data type")}
    ch <- res
    return fmt.Errorf("Unsupported raw data type")
}

//==========================================================================

func GetGroupSigsDataByRaw(raw string) (string,error) {
    if raw == "" {
	return "",fmt.Errorf("raw data empty")
    }
    
    tx := new(types.Transaction)
    raws := common.FromHex(raw)
    if err := rlp.DecodeBytes(raws, tx); err != nil {
	    return "",err
    }

    signer := types.NewEIP155Signer(big.NewInt(30400)) //
    _, err := types.Sender(signer, tx)
    if err != nil {
	return "",err
    }

    var threshold string
    var mode string
    var groupsigs string
    var groupid string

    req := TxDataReqAddr{}
    err = json.Unmarshal(tx.Data(), &req)
    if err == nil && req.TxType == "REQDCRMADDR" {
	threshold = req.ThresHold
	mode = req.Mode
	groupsigs = req.Sigs
	groupid = req.GroupId
    } else {
	rh := TxDataReShare{}
	err = json.Unmarshal(tx.Data(), &rh)
	if err == nil && rh.TxType == "RESHARE" {
	    threshold = rh.ThresHold
	    mode = rh.Mode
	    groupsigs = rh.Sigs
	    groupid = rh.GroupId
	}
    }

    if threshold == "" || mode == "" || groupid == "" {
	return "",fmt.Errorf("raw data error,it is not REQDCRMADDR tx or RESHARE tx")
    }

    if mode == "1" {
	return "",nil
    }

    if mode == "0" && groupsigs == "" {
	return "",fmt.Errorf("raw data error,must have sigs data when mode = 0")
    }

    nums := strings.Split(threshold, "/")
    nodecnt, _ := strconv.Atoi(nums[1])
    if nodecnt <= 1 {
	return "",fmt.Errorf("threshold error")
    }

    sigs := strings.Split(groupsigs,"|")
    //SigN = enode://xxxxxxxx@ip:portxxxxxxxxxxxxxxxxxxxxxx
    _, enodes := GetGroup(groupid)
    nodes := strings.Split(enodes, common.Sep2)
    if nodecnt != len(sigs) {
	return "",fmt.Errorf("group sigs error")
    }

    sstmp := strconv.Itoa(nodecnt)
    for j := 0; j < nodecnt; j++ {
	en := strings.Split(sigs[j], "@")
	for _, node := range nodes {
	    node2 := ParseNode(node)
	    enId := strings.Split(en[0],"//")
	    if len(enId) < 2 {
		return "",fmt.Errorf("group sigs error")
	    }

	    if strings.EqualFold(node2, enId[1]) {
		enodesigs := []rune(sigs[j])
		if len(enodesigs) <= len(node) {
		    return "",fmt.Errorf("group sigs error")
		}

		sig := enodesigs[len(node):]
		//sigbit, _ := hex.DecodeString(string(sig[:]))
		sigbit := common.FromHex(string(sig[:]))
		if sigbit == nil {
		    return "",fmt.Errorf("group sigs error")
		}

		pub,err := secp256k1.RecoverPubkey(crypto.Keccak256([]byte(node2)),sigbit)
		if err != nil {
		    return "",err
		}
		
		h := coins.NewCryptocoinHandler("FSN")
		if h != nil {
		    pubkey := hex.EncodeToString(pub)
		    from, err := h.PublicKeyToAddress(pubkey)
		    if err != nil {
			return "",err
		    }
		    
		    //5:eid1:acc1:eid2:acc2:eid3:acc3:eid4:acc4:eid5:acc5
		    sstmp += common.Sep
		    sstmp += node2
		    sstmp += common.Sep
		    sstmp += from
		}
	    }
	}
    }

    tmps := strings.Split(sstmp,common.Sep)
    if len(tmps) == (2*nodecnt + 1) {
	return sstmp,nil
    }

    return "",fmt.Errorf("group sigs error")
}

func CheckGroupEnode(gid string) bool {
    if gid == "" {
	return false
    }

    groupenode := make(map[string]bool)
    _, enodes := GetGroup(gid)
    nodes := strings.Split(enodes, common.Sep2)
    for _, node := range nodes {
	node2 := ParseNode(node)
	_, ok := groupenode[strings.ToLower(node2)]
	if ok {
	    return false
	}

	groupenode[strings.ToLower(node2)] = true
    }

    return true
}

//msg: key-enode:C1:X1:X2...:Xn
//msg: key-enode1:NoReciv:enode2:C1
func DisMsg(msg string) {
	defer func() {
	    if r := recover(); r != nil {
		fmt.Errorf("DisMsg Runtime error: %v\n%v", r, string(debug.Stack()))
		return
	    }
	}()

	if msg == "" {
	    return
	}

	//orderbook matchres
	mm := strings.Split(msg, common.Sep)
	if len(mm) < 3 {
		return
	}

	mms := mm[0]
	prexs := strings.Split(mms, "-")
	if len(prexs) < 2 {
		return
	}

	//msg:  hash-enode:C1:X1:X2
	w, err := FindWorker(prexs[0])
	if err != nil || w == nil {
	    mmtmp := mm[0:2]
	    ss := strings.Join(mmtmp, common.Sep)
	    common.Debug("===============DisMsg,worker was not found,so save the msg (c1 or accept res) to C1Data map=============","ss",strings.ToLower(ss),"msg",msg,"key",prexs[0])
	    C1Data.WriteMap(strings.ToLower(ss),msg)

	    return
	}

	msgCode := mm[1]
	switch msgCode {
	case "SyncPreSign":
		if w.msg_syncpresign.Len() >= w.ThresHold {
		    common.Debug("=============================DisMsg===============================","w.ThresHold",w.ThresHold,"w.msg_syncpresign.Len()",w.msg_syncpresign.Len(),"msgprex",prexs[0],"msg",msg)
			return
		}
		if Find(w.msg_syncpresign,msg) {
		    common.Debug("=============================DisMsg,found msg.===============================","w.ThresHold",w.ThresHold,"w.msg_syncpresign.Len()",w.msg_syncpresign.Len(),"msgprex",prexs[0],"msg",msg)
			return
		}

		w.msg_syncpresign.PushBack(msg)
		if w.msg_syncpresign.Len() == w.ThresHold {
			common.Debug("=============================DisMsg,set channel.===============================","w.ThresHold",w.ThresHold,"w.msg_syncpresign.Len()",w.msg_syncpresign.Len(),"msgprex",prexs[0],"msg",msg)
			w.bsyncpresign <- true
		}
	case "SendSignRes":
		///bug
		if w.msg_sendsignres.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_sendsignres, msg) {
			return
		}

		w.msg_sendsignres.PushBack(msg)
		if w.msg_sendsignres.Len() == w.ThresHold {
			w.bsendsignres <- true
		}
	case "SendReShareRes":
		///bug
		if w.msg_sendreshareres.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_sendreshareres, msg) {
			return
		}

		w.msg_sendreshareres.PushBack(msg)
		if w.msg_sendreshareres.Len() == w.NodeCnt {
			w.bsendreshareres <- true
		}
	case "C1":
		///bug
		if w.msg_c1.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_c1, msg) {
			return
		}

		w.msg_c1.PushBack(msg)
		if w.msg_c1.Len() == w.NodeCnt {
			common.Debug("======================DisMsg, Get All C1==================","w.msg_c1 len",w.msg_c1.Len(),"w.NodeCnt",w.NodeCnt,"key",prexs[0])
			w.bc1 <- true
		}
	case "D1":
		///bug
		if w.msg_d1_1.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_d1_1, msg) {
			return
		}

		w.msg_d1_1.PushBack(msg)
		if w.msg_d1_1.Len() == w.NodeCnt {
			w.bd1_1 <- true
		}
	case "SHARE1":
		///bug
		if w.msg_share1.Len() >= (w.NodeCnt-1) {
			return
		}
		///
		if Find(w.msg_share1, msg) {
			return
		}

		w.msg_share1.PushBack(msg)
		if w.msg_share1.Len() == (w.NodeCnt-1) {
			w.bshare1 <- true
		}
	//case "ZKFACTPROOF":
	case "NTILDEH1H2":
		///bug
		if w.msg_zkfact.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_zkfact, msg) {
			return
		}

		w.msg_zkfact.PushBack(msg)
		if w.msg_zkfact.Len() == w.NodeCnt {
			w.bzkfact <- true
		}
	case "ZKUPROOF":
		///bug
		if w.msg_zku.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_zku, msg) {
			return
		}

		w.msg_zku.PushBack(msg)
		if w.msg_zku.Len() == w.NodeCnt {
			w.bzku <- true
		}
	case "MTAZK1PROOF":
		///bug
		if w.msg_mtazk1proof.Len() >= (w.ThresHold-1) {
			return
		}
		///
		if Find(w.msg_mtazk1proof, msg) {
			return
		}

		w.msg_mtazk1proof.PushBack(msg)
		if w.msg_mtazk1proof.Len() == (w.ThresHold-1) {
			common.Debug("=====================Get All MTAZK1PROOF====================","key",prexs[0])
			w.bmtazk1proof <- true
		}
		//sign
	case "C11":
		///bug
		if w.msg_c11.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_c11, msg) {
			return
		}

		w.msg_c11.PushBack(msg)
		if w.msg_c11.Len() == w.ThresHold {
			common.Debug("=====================Get All C11====================","key",prexs[0])
			w.bc11 <- true
		}
	case "KC":
		///bug
		if w.msg_kc.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_kc, msg) {
			return
		}

		w.msg_kc.PushBack(msg)
		if w.msg_kc.Len() == w.ThresHold {
			common.Debug("=====================Get All KC====================","key",prexs[0])
			w.bkc <- true
		}
	case "MKG":
		///bug
		if w.msg_mkg.Len() >= (w.ThresHold-1) {
			return
		}
		///
		if Find(w.msg_mkg, msg) {
			return
		}

		w.msg_mkg.PushBack(msg)
		if w.msg_mkg.Len() == (w.ThresHold-1) {
			common.Debug("=====================Get All MKG====================","key",prexs[0])
			w.bmkg <- true
		}
	case "MKW":
		///bug
		if w.msg_mkw.Len() >= (w.ThresHold-1) {
			return
		}
		///
		if Find(w.msg_mkw, msg) {
			return
		}

		w.msg_mkw.PushBack(msg)
		if w.msg_mkw.Len() == (w.ThresHold-1) {
			common.Debug("=====================Get All MKW====================","key",prexs[0])
			w.bmkw <- true
		}
	case "DELTA1":
		///bug
		if w.msg_delta1.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_delta1, msg) {
			return
		}

		w.msg_delta1.PushBack(msg)
		if w.msg_delta1.Len() == w.ThresHold {
			common.Debug("=====================Get All DELTA1====================","key",prexs[0])
			w.bdelta1 <- true
		}
	case "D11":
		///bug
		if w.msg_d11_1.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_d11_1, msg) {
			return
		}

		w.msg_d11_1.PushBack(msg)
		if w.msg_d11_1.Len() == w.ThresHold {
			common.Debug("=====================Get All D11====================","key",prexs[0])
			w.bd11_1 <- true
		}
	case "CommitBigVAB":
		///bug
		if w.msg_commitbigvab.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_commitbigvab, msg) {
			return
		}

		w.msg_commitbigvab.PushBack(msg)
		if w.msg_commitbigvab.Len() == w.ThresHold {
			common.Debug("=====================Get All CommitBigVAB====================","key",prexs[0])
			w.bcommitbigvab <- true
		}
	case "ZKABPROOF":
		///bug
		if w.msg_zkabproof.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_zkabproof, msg) {
			return
		}

		w.msg_zkabproof.PushBack(msg)
		if w.msg_zkabproof.Len() == w.ThresHold {
			common.Debug("=====================Get All ZKABPROOF====================","key",prexs[0])
			w.bzkabproof <- true
		}
	case "CommitBigUT":
		///bug
		if w.msg_commitbigut.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_commitbigut, msg) {
			return
		}

		w.msg_commitbigut.PushBack(msg)
		if w.msg_commitbigut.Len() == w.ThresHold {
			common.Debug("=====================Get All CommitBigUT====================","key",prexs[0])
			w.bcommitbigut <- true
		}
	case "CommitBigUTD11":
		///bug
		if w.msg_commitbigutd11.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_commitbigutd11, msg) {
			return
		}

		w.msg_commitbigutd11.PushBack(msg)
		if w.msg_commitbigutd11.Len() == w.ThresHold {
			common.Debug("=====================Get All CommitBigUTD11====================","key",prexs[0])
			w.bcommitbigutd11 <- true
		}
	case "S1":
		///bug
		if w.msg_s1.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_s1, msg) {
			return
		}

		w.msg_s1.PushBack(msg)
		if w.msg_s1.Len() == w.ThresHold {
			common.Debug("=====================Get All S1====================","key",prexs[0])
			w.bs1 <- true
		}
	case "SS1":
		///bug
		if w.msg_ss1.Len() >= w.ThresHold {
			return
		}
		///
		if Find(w.msg_ss1, msg) {
			return
		}

		w.msg_ss1.PushBack(msg)
		if w.msg_ss1.Len() == w.ThresHold {
			common.Info("=====================Get All SS1====================","key",prexs[0])
			w.bss1 <- true
		}
	case "PaillierKey":
		///bug
		if w.msg_paillierkey.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_paillierkey, msg) {
			return
		}

		w.msg_paillierkey.PushBack(msg)
		//if w.msg_paillierkey.Len() == w.ThresHold {
		if w.msg_paillierkey.Len() == w.NodeCnt {
			common.Debug("=====================Get All PaillierKey====================","key",prexs[0])
			w.bpaillierkey <- true
		}


	//////////////////ed
	case "EDC11":
		///bug
		if w.msg_edc11.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edc11, msg) {
			return
		}

		w.msg_edc11.PushBack(msg)
		if w.msg_edc11.Len() == w.NodeCnt {
			w.bedc11 <- true
		}
	case "EDZK":
		///bug
		if w.msg_edzk.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edzk, msg) {
			return
		}

		w.msg_edzk.PushBack(msg)
		if w.msg_edzk.Len() == w.NodeCnt {
			w.bedzk <- true
		}
	case "EDD11":
		///bug
		if w.msg_edd11.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edd11, msg) {
			return
		}

		w.msg_edd11.PushBack(msg)
		if w.msg_edd11.Len() == w.NodeCnt {
			w.bedd11 <- true
		}
	case "EDSHARE1":
		///bug
		if w.msg_edshare1.Len() >= (w.NodeCnt-1) {
			return
		}
		///
		if Find(w.msg_edshare1, msg) {
			return
		}

		w.msg_edshare1.PushBack(msg)
		if w.msg_edshare1.Len() == (w.NodeCnt-1) {
			w.bedshare1 <- true
		}
	case "EDCFSB":
		///bug
		if w.msg_edcfsb.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edcfsb, msg) {
			return
		}

		w.msg_edcfsb.PushBack(msg)
		if w.msg_edcfsb.Len() == w.NodeCnt {
			w.bedcfsb <- true
		}
	case "EDC21":
		///bug
		if w.msg_edc21.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edc21, msg) {
			return
		}

		w.msg_edc21.PushBack(msg)
		if w.msg_edc21.Len() == w.NodeCnt {
			w.bedc21 <- true
		}
	case "EDZKR":
		///bug
		if w.msg_edzkr.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edzkr, msg) {
			return
		}

		w.msg_edzkr.PushBack(msg)
		if w.msg_edzkr.Len() == w.NodeCnt {
			w.bedzkr <- true
		}
	case "EDD21":
		///bug
		if w.msg_edd21.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edd21, msg) {
			return
		}

		w.msg_edd21.PushBack(msg)
		if w.msg_edd21.Len() == w.NodeCnt {
			w.bedd21 <- true
		}
	case "EDC31":
		///bug
		if w.msg_edc31.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edc31, msg) {
			return
		}

		w.msg_edc31.PushBack(msg)
		if w.msg_edc31.Len() == w.NodeCnt {
			w.bedc31 <- true
		}
	case "EDD31":
		///bug
		if w.msg_edd31.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_edd31, msg) {
			return
		}

		w.msg_edd31.PushBack(msg)
		if w.msg_edd31.Len() == w.NodeCnt {
			w.bedd31 <- true
		}
	case "EDS":
		///bug
		if w.msg_eds.Len() >= w.NodeCnt {
			return
		}
		///
		if Find(w.msg_eds, msg) {
			return
		}

		w.msg_eds.PushBack(msg)
		if w.msg_eds.Len() == w.NodeCnt {
			w.beds <- true
		}
		///////////////////
	default:
		fmt.Println("unkown msg code")
	}
}

//==========================================================================

func Find(l *list.List, msg string) bool {
	if l == nil || msg == "" {
		return false
	}

	var next *list.Element
	for e := l.Front(); e != nil; e = next {
		next = e.Next()

		if e.Value == nil {
			continue
		}

		s := e.Value.(string)

		if s == "" {
			continue
		}

		if strings.EqualFold(s, msg) {
			return true
		}
	}

	return false
}

func testEq(a, b []string) bool {
    // If one is nil, the other must also be nil.
    if (a == nil) != (b == nil) {
        return false;
    }

    if len(a) != len(b) {
        return false
    }

    for i := range a {
	if !strings.EqualFold(a[i],b[i]) {
            return false
        }
    }

    return true
}

