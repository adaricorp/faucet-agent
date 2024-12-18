package main

type FaucetEvent struct {
	Version      int           `json:"version"`
	Time         float64       `json:"time"`
	DpID         int           `json:"dp_id"`
	DpName       string        `json:"dp_name"`
	EventID      int           `json:"event_id"`
	ConfigChange *ConfigChange `json:"CONFIG_CHANGE,omitempty"`
	DpChange     *DpChange     `json:"DP_CHANGE,omitempty"`
	PortChange   *PortChange   `json:"PORT_CHANGE,omitempty"`
	L2Learn      *L2Learn      `json:"L2_LEARN,omitempty"`
	L3Learn      *L3Learn      `json:"L3_LEARN,omitempty"`
}

type ConfigChange struct {
	Success        *bool   `json:"success,omitempty"`
	RestartType    *string `json:"restart_type,omitempty"`
	ConfigHashInfo *struct {
		ConfigFiles string `json:"config_files"`
		Hashes      string `json:"hashes"`
		Error       string `json:"error"`
	} `json:"config_hash_info,omitempty"`
}

type DpChange struct {
	Reason string `json:"reason"`
}

type PortChange struct {
	PortNo int    `json:"port_no"`
	Reason string `json:"reason"`
	State  int    `json:"state"`
	Status bool   `json:"status"`
}

type L2Learn struct {
	PortNo         int         `json:"port_no"`
	PreviousPortNo interface{} `json:"previous_port_no"`
	Vid            int         `json:"vid"`
	EthSrc         string      `json:"eth_src"`
	EthDst         string      `json:"eth_dst"`
	EthType        int         `json:"eth_type"`
	L3SrcIP        string      `json:"l3_src_ip"`
	L3DstIP        string      `json:"l3_dst_ip"`
}

type L3Learn struct {
	EthSrc  string `json:"eth_src"`
	L3SrcIP string `json:"l3_src_ip"`
	PortNo  int    `json:"port_no"`
	Vid     int    `json:"vid"`
}
