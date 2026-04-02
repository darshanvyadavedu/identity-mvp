/**
 * The illumination states
 */
var IlluminationState;
(function (IlluminationState) {
    IlluminationState["DARK"] = "dark";
    IlluminationState["BRIGHT"] = "bright";
    IlluminationState["NORMAL"] = "normal";
})(IlluminationState || (IlluminationState = {}));
/**
 * The detected face states with respect to the liveness oval
 */
var FaceMatchState;
(function (FaceMatchState) {
    FaceMatchState["MATCHED"] = "MATCHED";
    FaceMatchState["TOO_FAR"] = "TOO FAR";
    FaceMatchState["CANT_IDENTIFY"] = "CANNOT IDENTIFY";
    FaceMatchState["FACE_IDENTIFIED"] = "ONE FACE IDENTIFIED";
    FaceMatchState["TOO_MANY"] = "TOO MANY FACES";
    FaceMatchState["OFF_CENTER"] = "OFF CENTER";
})(FaceMatchState || (FaceMatchState = {}));

export { FaceMatchState, IlluminationState };
