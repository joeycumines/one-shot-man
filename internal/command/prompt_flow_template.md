!! N.B. only statements surrounded by "!!" are _instructions_. !!

!! Generate a prompt using the template for purposes of achieving the following (higher level, less specific) goal. !!

!! Consider the implementation, attached, in doing so. !!

!! Plan for the entire "IMPLEMENTATIONS/CONTEXT" section to be attached after the prompt (with heading). !!

!! **GOAL:** !!

!! **IMPLEMENTATIONS/CONTEXT:** !!

!! **TEMPLATE:** !!
``````
**Add metrics for virtual thread based send operations, and a gauge or similar to indicate queued threads for the semaphore, in addition to metrics devised by you, precisely per my requirements, below.**

Core constraint: Don't deviate from the carefully crafted specifics of the existing implementation. Do not modify anything unnecessary, _especially_ if you don't understand why it exists. Don't modify or remove my existing comments or documentation, either.

CHANGE NO OTHER BEHAVIOR OTHER THAN SPECIFICALLY REQUESTED.

You must track the number of "running" or active `producer.sendEvents` calls
You pick the most appropriate name - align with standards, conventions, etc, and consider existing metrics.
Increment the gauge _within_ the Runnable passed to `senderExecutor.submit`.
Ensure correct synchronisation.
Decrement just before releasing the semaphore.

You must track the duration of `producer.sendEvent` calls. Use a histogram with appropriate buckets.

You must also track the count of `producer.sendEvent` calls.

The duration and count metrics MUST use the label `status` with the value of either `success` or `failure`. Attach this label to all existing or new metrics where it is relevant.

Think deeply about what is actually the correct and desired behavior here.

You MUST be able to guarantee that the solution is fit for purpose.
``````

!! **CRITICAL: If you do not rewrite the template IN FULL or fail to retain the STRUCTURE of the template AS CLOSE AS POSSIBLE per my SPECIFIC ASK, you have failed. N.B. The rewrite may (and should, if suitable) entail the same content as originally, for _some_ sections.** !!

!! **CRITICAL: If you do not rewrite the template IN FULL or fail to retain the STRUCTURE of the template AS CLOSE AS POSSIBLE per my SPECIFIC ASK, you have failed. N.B. The rewrite may (and should, if suitable) entail the same content as originally, for _some_ sections.** !!