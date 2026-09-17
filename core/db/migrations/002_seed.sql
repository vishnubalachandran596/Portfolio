-- Seed content sourced from Vishnu Balachandran's own resume/profile data.
-- Editable afterwards from the admin portal.

INSERT INTO profile (
    id, name, title, tagline, hero_subtitle, summary, location, email,
    linkedin_url, github_url, resume_url, photo_url, years_experience
) VALUES (
    1,
    'Vishnu Balachandran',
    'Technical Lead · Technical Architect · Staff Engineer · Engineering Manager (Track)',
    'I build resilient systems and lead teams that ship.',
    'Technical leader with 8+ years delivering resilient banking and fintech platforms, scalable architecture, faster delivery, and stronger engineering teams.',
    'I''m Vishnu Balachandran, a technology leader based in Kollam, Kerala, with more than eight years of experience building and scaling software for banking, fintech, travel, and commerce. I currently serve as Lead Product Engineer I at Zafin, where I architect event-driven pricing and billing platforms, guide engineering teams, and lead production delivery for banking clients.

My work sits at the intersection of architecture, delivery, and hands-on engineering. I have designed high-throughput systems processing millions of transactions and requests daily, improved reliability and deployment speed, reduced latency, and strengthened platform security. I also lead technical upgrades and mentoring, with experience guiding teams of 10 to 30 or more engineers.

I bring deep expertise in Java, Kotlin, Spring Boot, microservices, Kafka, cloud platforms, and AI-assisted engineering. My strongest domain is banking and fintech, especially pricing, billing, and deal management.

I''m focused on Technical Lead, Technical Architect, Staff Engineer, and Engineering Manager opportunities where I can shape resilient systems, raise engineering standards, and turn complex business problems into reliable products.',
    'Kollam, Kerala, India',
    '',
    'https://linkedin.com/in/vishnu-balachandran',
    '',
    '',
    '',
    8
) ON CONFLICT (id) DO NOTHING;

INSERT INTO home_features (title, description, sort_order) VALUES
    ('Courses I Teach', 'Practical lessons on architecture, backend engineering, cloud systems, and technical leadership.', 1),
    ('Book a 1:1 Session', 'Get focused guidance on engineering leadership, architecture, career growth, and delivery.', 2),
    ('Engineering Case Studies', 'Explore real challenges, decisions, and measurable outcomes from my production engineering work.', 3),
    ('Curated Resources', 'Read technical resources selected and organized by the site administrator.', 4)
ON CONFLICT DO NOTHING;

INSERT INTO skill_groups (category, level, items, sort_order) VALUES
    ('Distributed Systems & Architecture', 'Expert', 'Microservices architecture,Event-driven architecture (Kafka/Redpanda),DDD / CQRS / Clean Architecture,System design,Design patterns (GoF),REST API design,API Gateway', 1),
    ('Languages & Frameworks', 'Expert', 'Java 21,Kotlin,Spring Boot,Spring Cloud,Spring Batch,Spring Data JPA,Hibernate,Resilience4j,Netflix OSS', 2),
    ('Domain: Banking & Fintech', 'Expert', 'Pricing engines,Billing & invoicing,Deal & arrangement management,Profitability analysis,AUM calculations,Portfolio & custody operations', 3),
    ('Leadership & Mentorship', 'Expert', 'Technical leadership,Engineer mentorship,Stakeholder & executive communication,Agile / Scrum,Cross-functional team leadership (10-30+ engineers)', 4),
    ('Cloud & Platform', 'Advanced', 'Azure (AKS, DevOps, FlexiDB, CrunchyDB),AWS (EC2, EKS, S3),Docker,Kubernetes,ArgoCD,GitHub Actions,Jenkins,CI/CD & GitOps', 5),
    ('Data & Messaging', 'Advanced', 'Kafka,Redis,Elasticsearch,MongoDB,PostgreSQL,Oracle,MySQL,Azure CosmosDB,Apache NiFi,Ignite Cache', 6),
    ('API & Integration', 'Advanced', 'GraphQL API layer design,AI-assisted / agentic engineering,Observability (Prometheus, Grafana, ELK)', 7),
    ('Workflow & Frontend', 'Proficient', 'Temporal.io workflow orchestration,React,TypeScript', 8)
ON CONFLICT DO NOTHING;

INSERT INTO experiences (company, role, location, start_date, end_date, bullets, sort_order) VALUES
    ('Zafin', 'Lead Product Engineer I', 'Trivandrum, India', 'Mar 2025', 'Present',
     'Architected the Corporate Pricing Engine (CPE), an event-driven pricing/billing platform (Spring Boot, Kafka, PostgreSQL, DDD) processing 2M+ transactions/day at sub-second latency
Led delivery mentoring 10+ engineers; drove SOLID/CQRS adoption, cutting defect leakage 40%
Built delta-processing ETL (Spring Batch, Kafka partitioning) cutting data-load time 60%
Led the first production go-live of Deal Manager for ING Bank as technical SPOC, with 99.9% uptime on day one
Led a Spring Boot upgrade across 20+ microservices, eliminating 15+ CVEs
Built an AI-powered Pricing & Billing Copilot chatbot that cut L1 resolution time 50%', 1),
    ('Way.com', 'Technical Lead', 'Remote', 'Aug 2022', 'Dec 2024',
     'Built a Netflix-scale API gateway handling 5M+ daily requests (Resilience4j, Redis, Spring Cloud Gateway) at 99.95% availability
Shipped 30+ features across Parking, Carwash, Way+, Roadside Assistance & CRM with partners including Amazon, getUpside, FuelRewards, Travelers, and Cure, driving $1M+ incremental ARR
Built real-time analytics with Kafka Streams, Elasticsearch, and Kibana
Partnered directly with the CEO and cross-functional stakeholders on technical strategy, cutting average feature time-to-market from 6 weeks to 3 weeks', 2),
    ('Udaan.com', 'Application Developer', 'Bengaluru, India', 'May 2021', 'Jul 2022',
     'Built a custom API gateway on Azure with Resilience4j, cutting P99 latency 35%
Built a canary deployment framework with A/B testing and feature flags, cutting deployment incidents 70% and doubling release velocity
Built monitoring dashboards (Azure Data Explorer, Prometheus, Thanos, Grafana) cutting mean-time-to-detect from 15 minutes to under 3
Implemented a Resilience4j resiliency framework in the core API server, improving uptime from 99.5% to 99.95%', 3),
    ('SunTec Business Solutions', 'Software Engineer', 'Trivandrum, India', 'May 2019', 'May 2021',
     'Built an automated test framework for the Xelerate banking platform, cutting regression cycle from 5 days to 8 hours
Built Kafka/NiFi/Ignite Cache streaming pipelines for banking billing and payments
Created internal AWS deployment automation (Kubernetes, EKS), cutting manual deployment effort 80%
Added banking functionality (payment follow-up, bill generation) to Xelerate 2.5', 4),
    ('FlyTxt Mobile Solutions', 'Software Engineer R&D', 'Trivandrum, India', 'Jul 2018', 'May 2019',
     'Achieved a 10x latency reduction on a core platform component via JProfiler profiling and Hibernate optimization
Contributed to telecom/CRM platform engineering on a large-scale data pipeline', 5)
ON CONFLICT DO NOTHING;

INSERT INTO education (institution, degree, field, start_year, end_year, details, sort_order) VALUES
    ('TKM College of Engineering', 'B.Tech', 'Computer Science & Engineering', 2014, 2018, 'CGPA 7.7', 1)
ON CONFLICT DO NOTHING;

INSERT INTO courses (slug, title, summary, description, level, duration, price_cents, currency, is_published, sort_order) VALUES
    ('event-driven-microservices', 'Event-Driven Microservices with Spring Boot & Kafka',
     'Design and build production-grade event-driven systems from first principles.',
     'A practical course on architecting event-driven microservices: service boundaries with DDD, Kafka topic design and partitioning strategy, CQRS, idempotency, and observability. Grounded in patterns used to build platforms processing millions of transactions a day.',
     'Intermediate', '6 weeks', 0, 'USD', TRUE, 1),
    ('resilient-system-design', 'Designing Resilient Systems: Architecture for Scale',
     'Architecture patterns for reliability, resiliency, and graceful degradation at scale.',
     'Covers circuit breakers and bulkheads with Resilience4j, canary deployments, API gateway design, caching strategy, and the operational practices that keep uptime above 99.9% in production banking and travel-tech platforms.',
     'Advanced', '4 weeks', 0, 'USD', TRUE, 2),
    ('engineering-leadership', 'Engineering Leadership for Technical Leads',
     'Moving from senior engineer to technical lead: delivery, mentorship, and stakeholder trust.',
     'For engineers stepping into technical leadership: leading teams of 10-30+ engineers, running production go-lives, mentoring, and communicating architecture decisions to executive stakeholders.',
     'All levels', '3 weeks', 0, 'USD', TRUE, 3),
    ('ai-assisted-engineering', 'AI-Assisted Engineering with Claude, Cursor & Codex',
     'A practical playbook for shipping faster with AI coding agents in real production codebases.',
     'How to structure a codebase, review process, and multi-agent workflow so AI coding assistants like Claude, Cursor, and Codex genuinely speed up delivery instead of creating cleanup work.',
     'Intermediate', '2 weeks', 0, 'USD', TRUE, 4)
ON CONFLICT (slug) DO NOTHING;

-- Default availability: weekday evenings, in the site's local timezone.
INSERT INTO availability_rules (weekday, start_time, end_time, slot_minutes, active) VALUES
    (1, '18:00', '20:00', 30, TRUE),
    (2, '18:00', '20:00', 30, TRUE),
    (3, '18:00', '20:00', 30, TRUE),
    (4, '18:00', '20:00', 30, TRUE),
    (6, '10:00', '13:00', 30, TRUE)
ON CONFLICT DO NOTHING;
