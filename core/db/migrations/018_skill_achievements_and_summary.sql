-- Each skill category gets a one-line "what I achieve with this" so the
-- About/Home skills grid reads as real, grounded accomplishments rather
-- than a bare tag list. Folds in the content that used to live in a
-- separate "Major projects & impact" section (now removed from the page).
ALTER TABLE skill_groups ADD COLUMN IF NOT EXISTS achievement TEXT NOT NULL DEFAULT '';

UPDATE skill_groups SET achievement = 'Architected the event-driven pricing and billing platform powering production billing for banking clients at Zafin — the domain model, event flows, and service boundaries the team builds on daily.' WHERE category = 'Distributed Systems & Architecture';
UPDATE skill_groups SET achievement = 'Built and hardened services processing millions of transactions and requests daily, improving reliability, cutting latency, and speeding up deployments across the stack.' WHERE category = 'Languages & Frameworks';
UPDATE skill_groups SET achievement = '8+ years deep in banking and fintech platforms — pricing, billing, and deal management systems that real institutions run production revenue through.' WHERE category = 'Domain: Banking & Fintech';
UPDATE skill_groups SET achievement = 'Led and mentored engineering teams of 10 to 30+, raising delivery standards and growing engineers into architecture-level thinkers.' WHERE category = 'Leadership & Mentorship';
UPDATE skill_groups SET achievement = 'Ran production deployments and CI/CD pipelines on Azure and AWS for banking-grade platforms, closing security gaps without disrupting live traffic.' WHERE category = 'Cloud & Platform';
UPDATE skill_groups SET achievement = 'Designed the data and messaging backbone for event-driven systems handling banking-scale transaction volume, from Kafka topics to the databases behind them.' WHERE category = 'Data & Messaging';
UPDATE skill_groups SET achievement = 'Designed the API layers internal and client teams build against, plus the observability stack that keeps them accountable in production.' WHERE category = 'API & Integration';
UPDATE skill_groups SET achievement = 'Used Temporal for durable workflow orchestration and built the React/TypeScript interfaces product teams actually touch.' WHERE category = 'Workflow & Frontend';

-- A tighter, more technical, keyword-dense professional summary — the
-- old one ran long across several loosely-connected sentences.
UPDATE profile SET summary =
'I''m Vishnu Balachandran, a technical consultant and Java technical expert with 8+ years architecting resilient banking and fintech platforms. I currently lead event-driven pricing and billing systems at Zafin — Java, Kotlin, Spring Boot, microservices, and Kafka are the stack I build and run production infrastructure on.

My work spans technical architecture, system design, and engineering leadership: high-throughput systems processing millions of transactions daily, platform security hardening, and mentoring engineering teams of 10 to 30+ into stronger architects. I take on Technical Lead, Staff Engineer, and consulting engagements where the problem is genuinely hard.'
WHERE id = 1;
