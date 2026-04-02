import React__default from 'react';
import { LivenessClassNames } from '../types/classNames.mjs';

const MatchIndicator = ({ percentage, initialPercentage = 25, testId, }) => {
    const [matchPercentage, setMatchPercentage] = React__default.useState(initialPercentage);
    React__default.useEffect(() => {
        if (percentage < 0) {
            setMatchPercentage(0);
        }
        else if (percentage > 100) {
            setMatchPercentage(100);
        }
        else {
            setMatchPercentage(percentage);
        }
    }, [percentage]);
    const percentageStyles = {
        '--percentage': `${matchPercentage}%`,
    };
    return (React__default.createElement("div", { className: LivenessClassNames.MatchIndicator, "data-testid": testId },
        React__default.createElement("div", { className: `${LivenessClassNames.MatchIndicator}__bar`, style: percentageStyles, role: "progressbar", "aria-label": "MatchIndicator", "aria-valuenow": percentage, "aria-valuetext": `${percentage}% face fit` })));
};

export { MatchIndicator };
