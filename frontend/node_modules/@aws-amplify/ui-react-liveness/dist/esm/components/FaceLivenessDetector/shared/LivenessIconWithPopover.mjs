import * as React from 'react';
import { Button, Flex } from '@aws-amplify/ui-react';
import { useThemeBreakpoint, AlertIcon } from '@aws-amplify/ui-react/internal';
import { LivenessClassNames } from '../types/classNames.mjs';

/**
 * Copied from src/primitives/Alert/AlertIcon.tsx because we want to re-use the icon but it is not currently expored by AlertIcon.
 * We currently don't want to make a change to the AlertIcon primitive itself and may expose the icon in the future but for now so as not to introduce cross component dependencies we have duplicated it.
 */
const LivenessIconWithPopover = ({ children, headingText, labelText }) => {
    const breakpoint = useThemeBreakpoint();
    const [shouldShowPopover, setShouldShowPopover] = React.useState(false);
    const wrapperRef = React.useRef(null);
    const isMobileScreen = breakpoint === 'base';
    React.useEffect(() => {
        function handleClickOutside(event) {
            if (shouldShowPopover &&
                wrapperRef.current &&
                !wrapperRef.current.contains(event.target)) {
                setShouldShowPopover(false);
            }
        }
        document.addEventListener('mousedown', handleClickOutside);
        return () => {
            document.removeEventListener('mousedown', handleClickOutside);
        };
    }, [wrapperRef, shouldShowPopover]);
    return (React.createElement("div", { className: LivenessClassNames.Popover, ref: wrapperRef },
        React.createElement(Button, { "aria-controls": "photosensitivity-description", "aria-expanded": shouldShowPopover, role: "alertdialog", "aria-label": labelText, "aria-describedby": "photosensitivity-description", colorTheme: "info", id: "popover-button", onClick: () => setShouldShowPopover(!shouldShowPopover), testId: "popover-icon" },
            React.createElement(AlertIcon, { ariaHidden: true, variation: "info" })),
        shouldShowPopover && (React.createElement(React.Fragment, null,
            React.createElement(Flex, { className: LivenessClassNames.PopoverAnchor }),
            React.createElement(Flex, { className: LivenessClassNames.PopoverAnchorSecondary }),
            React.createElement(Flex, { "aria-hidden": !shouldShowPopover, "aria-label": headingText, className: LivenessClassNames.PopoverContainer, "data-testid": "popover-text", id: "photosensitivity-description", left: isMobileScreen ? -190 : -108, role: "alertdialog" }, children)))));
};
LivenessIconWithPopover.displayName = 'LivenessIconWithPopover';

export { LivenessIconWithPopover };
