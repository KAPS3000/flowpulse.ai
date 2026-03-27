// SPDX-License-Identifier: GPL-2.0
// FlowPulse - TC egress/ingress flow tracker with RDMA header parsing

#include "vmlinux.h"
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include "common.h"

#ifndef ETH_P_IP
#define ETH_P_IP 0x0800
#endif

#ifndef TC_ACT_OK
#define TC_ACT_OK 0
#endif

char LICENSE[] SEC("license") = "GPL";

/* ── Maps ──────────────────────────────────────────────────── */

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_FLOWS);
    __type(key, struct flow_key);
    __type(value, struct flow_value);
} flow_table SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_RINGBUF);
    __uint(max_entries, RINGBUF_SIZE);
} flow_events SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, __u64);
} pkt_counter SEC(".maps");

/* ── Helpers ───────────────────────────────────────────────── */

static __always_inline int parse_eth_ip(struct __sk_buff *skb,
                                         struct flow_key *key,
                                         __u32 *payload_offset) {
    __u16 h_proto;
    if (bpf_skb_load_bytes(skb, offsetof(struct ethhdr, h_proto),
                           &h_proto, sizeof(h_proto)) < 0)
        return -1;

    if (h_proto != bpf_htons(ETH_P_IP))
        return -1;

    __u32 eth_len = sizeof(struct ethhdr);
    struct iphdr ip;
    if (bpf_skb_load_bytes(skb, eth_len, &ip, sizeof(ip)) < 0)
        return -1;

    key->src_ip   = bpf_ntohl(ip.saddr);
    key->dst_ip   = bpf_ntohl(ip.daddr);
    key->protocol = ip.protocol;

    __u32 ip_hdr_len = ip.ihl * 4;
    if (ip_hdr_len < 20 || ip_hdr_len > 60)
        return -1;

    __u32 l4_off = eth_len + ip_hdr_len;

    if (ip.protocol == IPPROTO_TCP) {
        struct tcphdr tcp;
        if (bpf_skb_load_bytes(skb, l4_off, &tcp, sizeof(tcp)) < 0)
            return -1;
        key->src_port = bpf_ntohs(tcp.source);
        key->dst_port = bpf_ntohs(tcp.dest);
        *payload_offset = l4_off + tcp.doff * 4;
    } else if (ip.protocol == IPPROTO_UDP) {
        struct udphdr udp;
        if (bpf_skb_load_bytes(skb, l4_off, &udp, sizeof(udp)) < 0)
            return -1;
        key->src_port = bpf_ntohs(udp.source);
        key->dst_port = bpf_ntohs(udp.dest);
        *payload_offset = l4_off + sizeof(struct udphdr);
    } else {
        key->src_port = 0;
        key->dst_port = 0;
        *payload_offset = l4_off;
    }

    return 0;
}

/* RoCEv2 uses UDP dst port 4791 */
#define ROCEV2_UDP_PORT 4791

static __always_inline void try_parse_rdma(struct __sk_buff *skb,
                                            struct flow_key *key,
                                            struct flow_value *val,
                                            __u32 payload_offset) {
    if (key->protocol != IPPROTO_UDP || key->dst_port != ROCEV2_UDP_PORT)
        return;

    struct bth_header bth;
    if (bpf_skb_load_bytes(skb, payload_offset, &bth, sizeof(bth)) < 0)
        return;

    val->rdma_qp      = bpf_ntohl(bth.dest_qp) & 0x00FFFFFF;
    val->rdma_msg_count++;

    if (bth.flags & 0x80)
        val->rdma_ecn_marks++;
}

static __always_inline void update_flow(struct __sk_buff *skb,
                                         struct flow_key *key,
                                         __u8 direction,
                                         __u32 payload_offset) {
    __u64 now = bpf_ktime_get_ns();
    __u32 pkt_len = skb->len;

    struct flow_value *val = bpf_map_lookup_elem(&flow_table, key);
    if (val) {
        __sync_fetch_and_add(&val->packets, 1);
        __sync_fetch_and_add(&val->bytes, pkt_len);
        val->last_seen_ns = now;
        try_parse_rdma(skb, key, val, payload_offset);
    } else {
        struct flow_value new_val = {};
        new_val.packets       = 1;
        new_val.bytes         = pkt_len;
        new_val.first_seen_ns = now;
        new_val.last_seen_ns  = now;
        new_val.direction     = direction;

        try_parse_rdma(skb, key, &new_val, payload_offset);

        bpf_map_update_elem(&flow_table, key, &new_val, BPF_NOEXIST);

        /* Notify userspace of new flow via ring buffer */
        struct flow_event *evt = bpf_ringbuf_reserve(&flow_events,
                                                      sizeof(*evt), 0);
        if (evt) {
            __builtin_memcpy(&evt->key, key, sizeof(*key));
            evt->direction    = direction;
            evt->timestamp_ns = now;
            bpf_ringbuf_submit(evt, 0);
        }
    }

    /* Increment global packet counter */
    __u32 zero = 0;
    __u64 *cnt = bpf_map_lookup_elem(&pkt_counter, &zero);
    if (cnt)
        __sync_fetch_and_add(cnt, 1);
}

/* ── TC hooks ──────────────────────────────────────────────── */

SEC("tc/ingress")
int flowpulse_ingress(struct __sk_buff *skb) {
    struct flow_key key = {};
    __u32 payload_offset = 0;

    if (parse_eth_ip(skb, &key, &payload_offset) < 0)
        return TC_ACT_OK;

    update_flow(skb, &key, 1 /* ingress */, payload_offset);
    return TC_ACT_OK;
}

SEC("tc/egress")
int flowpulse_egress(struct __sk_buff *skb) {
    struct flow_key key = {};
    __u32 payload_offset = 0;

    if (parse_eth_ip(skb, &key, &payload_offset) < 0)
        return TC_ACT_OK;

    update_flow(skb, &key, 2 /* egress */, payload_offset);
    return TC_ACT_OK;
}
