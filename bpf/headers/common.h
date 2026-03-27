#ifndef __FLOWPULSE_COMMON_H
#define __FLOWPULSE_COMMON_H

/* Shared data structures between eBPF C programs and Go userspace.
 * Field sizes and layouts must match the Go mirror structs exactly. */

#define MAX_FLOWS       1000000
#define MAX_CPUS        1024
#define FLOW_TIMEOUT_NS 30000000000ULL  /* 30 seconds */
#define RINGBUF_SIZE    (1 << 24)       /* 16 MB ring buffer */

/* ── Flow tracking ─────────────────────────────────────────── */

struct flow_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8  protocol;
    __u8  pad[3];
} __attribute__((packed));

struct flow_value {
    __u64 packets;
    __u64 bytes;
    __u64 first_seen_ns;
    __u64 last_seen_ns;
    __u8  direction;     /* 1=ingress, 2=egress */
    __u8  pad[7];
    /* RDMA fields (populated when IB traffic detected) */
    __u32 rdma_qp;
    __u32 rdma_dest_qp;
    __u64 rdma_msg_count;
    __u64 rdma_retransmits;
    __u64 rdma_ecn_marks;
    __u64 rdma_cnp_count;
} __attribute__((packed));

/* Ring buffer event for new flow notifications */
struct flow_event {
    struct flow_key key;
    __u8  direction;
    __u8  pad[7];
    __u64 timestamp_ns;
} __attribute__((packed));

/* ── CPU scheduling ────────────────────────────────────────── */

struct cpu_key {
    __u32 cpu_id;
    __u64 cgroup_id;
} __attribute__((packed));

struct cpu_value {
    __u64 on_cpu_ns;
    __u64 off_cpu_ns;
    __u64 voluntary_switches;
    __u64 involuntary_switches;
    __u64 last_switch_ns;
} __attribute__((packed));

struct softirq_value {
    __u64 total_ns;
    __u64 net_rx_ns;    /* NET_RX softirq time */
    __u64 net_tx_ns;    /* NET_TX softirq time */
    __u64 count;
} __attribute__((packed));

/* Ring buffer event for CPU anomalies */
struct cpu_event {
    __u32 cpu_id;
    __u64 cgroup_id;
    __u64 timestamp_ns;
    __u8  event_type;   /* 0=high_util, 1=high_softirq, 2=stall */
    __u8  pad[3];
} __attribute__((packed));

/* ── InfiniBand verbs ──────────────────────────────────────── */

struct ib_op_key {
    __u32 qp_number;
    __u32 port;
} __attribute__((packed));

struct ib_op_value {
    __u64 send_count;
    __u64 recv_count;
    __u64 send_bytes;
    __u64 recv_bytes;
    __u64 completion_count;
    __u64 completion_latency_ns;
    __u64 error_count;
    __u64 last_op_ns;
} __attribute__((packed));

/* ── RDMA transport headers (IB/RoCEv2) ───────────────────── */

struct grh_header {
    __u32 version_tclass_flow;
    __u16 payload_len;
    __u8  next_header;
    __u8  hop_limit;
    __u8  src_gid[16];
    __u8  dst_gid[16];
} __attribute__((packed));

struct bth_header {
    __u8  opcode;
    __u8  flags;        /* solicited, migreq, pad_count, tver */
    __u16 pkey;
    __u32 dest_qp;      /* reserved(8) + dest_qp(24) */
    __u32 psn;          /* ack_req(1) + reserved(7) + psn(24) */
} __attribute__((packed));

#endif /* __FLOWPULSE_COMMON_H */
