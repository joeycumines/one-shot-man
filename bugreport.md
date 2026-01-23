You absolute disappointment - manual mode of scripts/example-05-pick-and-place.js fails to work properly.

It MUST be possible to TAKE CONTROL then later resume the PA-BT / auto mode.

Input handling MUST be robust and PERFORMANT - you must be able to HOLD DOWN KEYS and it MUST have GOOD, intuitive UX.

The simulation quality MUST be perfect - and PERFECTLY CONSISTENT across BOTH the manual mode and automatic mode.

You MUST _integration test_ ALL manual behavior AND the swapping between manual and automatic modes - prior to completing the goal. There MUST NOT BE HANGS.
**WORK DEFENSIVELY AGAINST HANGS - run hang-possible tests individually, and ALWAYS use appropriate timeouts (start short and work up as necessary).**
You can run arbitrary commands via config.mk dont forget.

DO NOT change the PA-BT behavior AT ALL - NO TOUCH.

ALL checks MUST pass - per ./config.mk (READ IT!).

THIS MUST ALL BE REPRESENTED IN YOUR blueprint.json
