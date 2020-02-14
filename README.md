# empi

This is a simple wrapper around the NHS Wales' enterprise master patient index (EMPI).

It is designed to simplify and abstract working with the EMPI, and isolate the integration component so that it can be developed independently from the main (client) application. It provides simple and crude logging and caching and will map magic internal codes into more standardised organisational references using a standard reference data approach / object identifier (OID) / namespace / URL to make consuming the API more straightforward.

