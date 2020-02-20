# Concierge

Concierge is a suite of health and care integration modules, abstracting and simplifing integrations with underlying health and care systems. 
	
> A concierge assists guests. This concierge assists clients to integrate into the local health and care ecosystem.

## Background

I built an electronic health record (2007-present day) and found that the most difficult part was integrations with other systems. Unlike many monolithic systems, I wanted to delegate important functionality to a suite of different platform services, so that I could concentrate on creating value. 

Unfortunately, I didn't find a suite of effective platform services, but instead many different  data and computing services. 

The core services I needed were:

* terminology - SNOMED-CT.
* reference data - from the NHS "organisational data services"
* demographics - patient identity
* staff identity
* appointments and scheduling
* document repositories

When I started, there wasn't an off-the-shelf service for terminology or reference data, but they were published via TRUD by NHS Digital. So I originally built these services into the wider EPR, but later realised the importance of spinning them out into their own microservices. You can try out my [terminology server](https://github.com/wardle/go-terminology).

As an EPR not linked to a single organisation, but instead providing services across organisational and disciplinary boundaries, I wanted to hide the underlying complexity from end-users. After all, the patient they are supporting is the same patient, whether it's via a telephone consultation, a GP surgery in Aberystwyth or an outpatient clinic at the University Hospital of Wales.

So I wrote interfaces to multiple services; the best way to describe this was a "war of attrition" with difficulties such as

* difficulty getting access with lots of discussions and interactions with lots of stakeholders
* seemingly arbitrary rules on access  
* no documentation, or out-of-date documentation
* no cohesive structure
* bespoke proprietary web service calls
* sometimes extraordinarily slow response times
* service endpoints that are ephemeral
* no authentication, and authorisation at the level of firewalls
* multiple firewalls, blocking both outgoing and incoming traffic
* logging by the client and not the service 
* no way to do development while actually on the network - so develop on laptop, compile and then use a USB stick to transfer for testing
* lack of readily-available on-demand test environments such as docker images / example source code.

These experiences, over many years, highlighted the need for a set of cohesive platform services - as [outlined on my personal blog](https://wardle.org). 

## Services

Concierge is a new lightweight microservice that provides integration services. It is a work-in-progress as I port across my existing (legacy) integrations.

It currently supports the following integrations:

* A wrapper around the NHS Wales' enterprise master patient index (EMPI).

The current development supports:

* Staff authentication and lookup via the national directory service (NADEX) to provide staff identity services

I have integrations with the following systems, that are due to be ported to this new application:

* Cardiff and Vale Patient Administative System (PAS) integration - for demographics and appointments/scheduling.
* Aneurin Bevan PAS integration
* Cardiff and Vale Document Repository service
* National document repository service (Welsh Care Records Service)

Future integrations that will be needed will be:

* Welsh Results Reporting Services (WRRS) - although currently permission has not been available to access this service
* Welsh Demographics Service (WDS) - which includes lookup via the NHS England Spine for NHS number tracing


# Module documentation


## EMPI - Welsh Enterprise Master Patient Index

This module is designed to simplify and abstract working with the EMPI, and isolate the integration component so that it can be developed independently from the main (client) application. It provides simple and crude logging and caching and will map magic internal codes into more standardised organisational references using a standard reference data approach / object identifier (OID) / namespace / URL to make consuming the API more straightforward.

