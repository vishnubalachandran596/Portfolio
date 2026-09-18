-- Final-quiz questions for each existing paid course, grounded in that
-- course's actual content sections (see core/content/courses/*.html).

-- go-portfolio-from-scratch (id 11)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(11, 'A Go function returning (int, error) signals what idiom?', 'Exceptions should be caught with try/catch', 'The caller must check the error before trusting the result', 'Errors are always fatal and stop the program', 'Only the first return value matters', 'B', 1),
(11, 'What is the difference between a value receiver and a pointer receiver on a Go method?', 'Value receivers can mutate the original struct; pointer receivers cannot', 'A pointer receiver can mutate the original struct; a value receiver operates on a copy', 'There is no difference in Go', 'Pointer receivers only work with interfaces', 'B', 2),
(11, 'How are Go interfaces typically satisfied?', 'Explicitly, with an "implements" keyword', 'Implicitly — any type with the matching methods satisfies the interface', 'Only structs can implement interfaces', 'Interfaces must be imported from the same package', 'B', 3),
(11, 'What does Go''s "comma-ok" idiom return on a map lookup?', 'Whether the map is nil', 'The value and a boolean indicating whether the key was present', 'The number of keys in the map', 'Whether the map is safe for concurrent access', 'B', 4),
(11, 'In net/http, what is http.HandlerFunc?', 'A struct that stores request data', 'An adapter type that lets an ordinary function satisfy the http.Handler interface', 'A built-in router', 'A middleware chaining library', 'B', 5);

-- java-fundamentals (id 6)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(6, 'What''s the key difference between == and .equals() for Java objects?', 'They always behave identically', '== compares references; .equals() compares what the class defines as meaningful equality', '.equals() is faster than ==', '== only works on primitives', 'B', 1),
(6, 'What does Java''s Collections Framework primarily standardize?', 'Only array operations', 'A unified set of interfaces and implementations for storing and manipulating groups of objects', 'Thread synchronization', 'File I/O', 'B', 2),
(6, 'Why were Generics introduced in Java?', 'To speed up compilation', 'To provide compile-time type safety and eliminate unchecked casts', 'To replace interfaces', 'To allow multiple inheritance', 'B', 3),
(6, 'In Java concurrency, what commonly causes a race condition?', 'Using final variables', 'Multiple threads reading and writing shared mutable state without proper synchronization', 'Using too many interfaces', 'Declaring a class abstract', 'B', 4),
(6, 'What does the JVM garbage collector primarily manage?', 'CPU scheduling', 'Automatic reclamation of heap memory no longer reachable from the program', 'Thread priority', 'Network sockets', 'B', 5);

-- java-sse-ultimate-guide (id 5)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(5, 'What data structures are combined to implement an LRU Cache in O(1) time?', 'Array and binary search', 'A hash map and a doubly linked list', 'A single stack', 'A min-heap only', 'B', 1),
(5, 'The "Course Schedule" problem (can all courses be finished given prerequisites) is fundamentally which graph problem?', 'Shortest path', 'Cycle detection / topological sort on a directed graph', 'Minimum spanning tree', 'Union-Find only', 'B', 2),
(5, 'What technique does the classic "3Sum" solution use to beat brute-force O(n^3)?', 'Brute-force triple nested loop only', 'Sort the array, then use two pointers for each fixed element', 'Dynamic programming', 'Binary search on the answer', 'B', 3),
(5, 'For "Daily Temperatures," which data structure gives an efficient O(n) solution?', 'A monotonic stack', 'A priority queue', 'A trie', 'A union-find structure', 'A', 4),
(5, 'A Segment Tree, as used for "Range Sum Query with Updates," is chosen mainly because it supports:', 'O(1) updates and O(n) queries', 'Both range queries and point updates in O(log n) time', 'Only static arrays with no updates', 'Constant-time range sum with no preprocessing', 'B', 5);

-- kafka-messaging-caching (id 9)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(9, 'In Kafka, what determines the message ordering guarantee?', 'Ordering is guaranteed across the entire topic', 'Ordering is only guaranteed within a single partition', 'Ordering is guaranteed only for compacted topics', 'Kafka provides no ordering guarantees at all', 'B', 1),
(9, 'What is a Kafka consumer offset?', 'The partition''s total message count', 'A pointer tracking how far a consumer group has read within a partition', 'The replication factor', 'The broker''s disk usage', 'B', 2),
(9, 'Which delivery semantic requires idempotent producers and transactional consumers?', 'At-most-once', 'Exactly-once', 'At-least-once', 'Best-effort', 'B', 3),
(9, 'What problem does the Transactional Outbox pattern solve?', 'Slow consumer throughput', 'The dual-write problem between a database update and a message publish', 'Partition rebalancing storms', 'Schema evolution', 'B', 4),
(9, 'What is the core difference between a KTable and a KStream in Kafka Streams?', 'They are identical', 'A KStream represents a stream of events; a KTable represents a continuously updated view keyed by the latest value', 'A KTable can''t be queried', 'A KStream is only for testing', 'B', 5);

-- payment-integration-razorpay (id 12)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(12, 'Why must a Razorpay payment''s amount always be resolved server-side rather than trusted from the client?', 'It''s not actually necessary since Razorpay validates it', 'A client-supplied amount could be tampered with to under-pay — server-side pricing prevents this', 'Client-side amounts are faster to process', 'Razorpay requires amounts in a different currency format', 'B', 1),
(12, 'What is the purpose of verifying the Razorpay payment signature server-side after checkout?', 'It''s optional and only for display', 'It''s the only step that cryptographically proves the payment actually happened and wasn''t spoofed by the client', 'It speeds up the checkout modal', 'It replaces the need for an order ID', 'B', 2),
(12, 'Why should a payment verify endpoint be idempotent (safe to call twice for the same order)?', 'Idempotency isn''t relevant to payments', 'A duplicate or retried verify call must never double-process the same payment', 'It reduces the KEY_SECRET length requirement', 'It disables Razorpay''s fraud checks', 'B', 3),
(12, 'In a pay-what-you-want pricing model, what must the server always enforce?', 'Nothing — the client''s chosen amount is always trusted', 'A minimum floor amount, regardless of what the client submits', 'A maximum ceiling only', 'A fixed price with no flexibility', 'B', 4),
(12, 'What is the security risk of embedding the Razorpay KEY_SECRET in frontend JavaScript?', 'There is no risk, it''s designed to be public', 'It would let anyone forge valid-looking signatures and compromise the integration', 'It would only slow down page load', 'It only affects test-mode payments', 'B', 5);

-- resilient-scalable-systems (id 10)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(10, 'What is "bulkhead isolation" in resilient system design?', 'Encrypting data at rest', 'Partitioning resources so a failure in one area can''t exhaust resources needed by others', 'A caching strategy', 'A load balancing algorithm', 'B', 1),
(10, 'What does a "retry storm" describe?', 'A single retry that always succeeds', 'Cascading, synchronized retries across many clients that overwhelm an already-struggling downstream service', 'A database backup process', 'A type of network partition', 'B', 2),
(10, 'In consensus algorithms like Raft or Paxos, what problem are they fundamentally solving?', 'Load balancing traffic', 'Getting a distributed set of nodes to agree on a single value/state despite failures', 'Compressing data for storage', 'Encrypting network traffic', 'B', 3),
(10, 'What do p50/p95/p99 latency figures represent?', 'Average latency only', 'Percentile latencies — e.g. p99 means 99% of requests were faster than this value', 'The slowest possible latency', 'Total request count', 'B', 4),
(10, 'Why is a "hot partition" a scaling problem for stateful systems?', 'It means the database is too cold to query', 'Uneven key distribution concentrates load on one partition/shard, creating a bottleneck', 'It only affects read replicas', 'It is not actually a real-world concern', 'B', 5);

-- system-design-deployment (id 8)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(8, 'What is the key difference between vertical and horizontal scaling?', 'Vertical scaling adds more machines; horizontal scaling adds power to one machine', 'Vertical scaling adds more power (CPU/RAM) to a single machine; horizontal scaling adds more machines', 'They are the same strategy with different names', 'Horizontal scaling only applies to databases', 'B', 1),
(8, 'What does the CAP theorem state about distributed systems?', 'A system can always guarantee Consistency, Availability, and Partition tolerance simultaneously', 'During a network partition, a system must choose between consistency and availability', 'CAP only applies to single-node databases', 'CAP theorem is about caching strategies', 'B', 2),
(8, 'What is a "cache stampede"?', 'A caching library crash', 'Many concurrent requests all missing the cache at once and hammering the backing store simultaneously', 'A type of cache eviction policy', 'A load balancer failure mode', 'B', 3),
(8, 'What''s the main trade-off of a "write-behind" (write-back) cache strategy?', 'It''s always safer than write-through', 'It improves write latency but risks data loss if the cache fails before flushing to the store', 'It eliminates the need for a database', 'It only works with read-heavy workloads', 'B', 4),
(8, 'What is a "dead-letter queue" used for?', 'Storing successfully processed messages permanently', 'Capturing messages that repeatedly fail processing so they don''t block the main queue', 'Load balancing between queues', 'Encrypting queue messages', 'B', 5);

-- technical-architecture-design (id 7)
INSERT INTO quiz_questions (course_id, question, option_a, option_b, option_c, option_d, correct_option, sort_order) VALUES
(7, 'What does the "S" in SOLID stand for?', 'Simplicity principle', 'Single Responsibility Principle — a class/module should have one reason to change', 'Scalability principle', 'State management principle', 'B', 1),
(7, 'What is the main goal of Hexagonal (Ports & Adapters) architecture?', 'To maximize database coupling', 'To isolate core domain logic from external concerns via defined ports and adapters', 'To eliminate the need for interfaces', 'To enforce a strict microservices topology', 'B', 2),
(7, 'What problem does the Outbox pattern solve?', 'Slow database reads', 'The dual-write problem — atomically persisting a state change and reliably publishing its event', 'API rate limiting', 'Schema versioning', 'B', 3),
(7, 'According to Conway''s Law, what tends to mirror an organization''s communication structure?', 'The company''s revenue', 'The system''s software architecture and service boundaries', 'The database schema only', 'The choice of programming language', 'B', 4),
(7, 'Why is idempotency described as "non-negotiable" in event-driven architecture?', 'Because messages are always delivered exactly once', 'Because at-least-once delivery is common, so handlers must safely process a repeated message', 'It is only relevant for synchronous APIs', 'It only matters for read operations', 'B', 5);
