# polycast

`go get github.com/jonathanmweiss/polycast`

polycast is an experimental implementation of Polynomial Reliable Broadcast, developed as part of a PhD course on distributed systems and cryptography.
This project demonstrates how polynomial-based techniques can reduce the communication overhead of classical reliable broadcast protocols — lowering bandwidth complexity from
O(n² · msgSize) to O(n² · log q), where q is the size of the ring used for polynomial commitments.

# Overview

Reliable broadcast is a key primitive in distributed systems, ensuring that all honest participants deliver the same message or not at all, despite the presence of malicious parties.


Traditional reliable broadcast protocols incur high bandwidth costs — typically `O(n² · msgSize)`.
polycast implements a Polynomial Reliable Broadcast protocol that leverages polynomial interpolation and evaluation techniques to reduce communication overhead, achieving
`O(n² · log q)` bandwidth, where q is the modulus of the ring over which polynomials are evaluated.

This implementation is designed for educational and experimental purposes, providing a clear and modular Go codebase for studying bandwidth-efficient reliable broadcast.


# Reporting security problems
This library is offered as-is, and without a guarantee. It will need an independent security review before it should be considered ready for use in security-critical applications. If you integrate Kyber into your application it is YOUR RESPONSIBILITY to arrange for that audit.