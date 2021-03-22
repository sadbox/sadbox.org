$(function () {
     $("#spinner").show();
     $("#spinnerTwo").show();
     $.getJSON(window.location.pathname + 'postsbyminute', function (data) {
        Highcharts.setOptions({
            global : {
                useUTC : false
            }
        });
        $("#spinner").hide();
        $("#postsByMinute").show();
        var currentime = new Date();
        $('#postsByMinute').highcharts({
            chart: {
                type: 'area'
            },
            title: {
                text: 'Activity in channel by time of day'
            },
            xAxis: {
                type: 'datetime',
                dateTimeLabelFormats: {
                    day: '%H:%M'
                },
                title: {
                    text: "Time of Post (UTC Offset: "+(-currentime.getTimezoneOffset()/60)+")"
                }
            },
            yAxis: {
                title: {
                    text: 'Posts Per Minute'
                }
            },
            tooltip: {
                formatter: function() {
                    return '<b>'+this.y.toPrecision(3)+'</b> posts per minute at <b>'+Highcharts.dateFormat('%H:%M', this.x)+'</b>'
                }
            },
            legend: {
                enabled: false
            },
            credits: {
                enabled: false
            },
            plotOptions: {
                area: {
                    pointStart: Date.UTC(0,0,0),
                    pointInterval: 60 * 1000,
                    marker: {
                        enabled: false,
                        symbol: 'circle',
                        radius: 2,
                        states: {
                            hover: {
                                enabled: true
                            }
                        }
                    }
                },
                spline: {
                    pointStart: Date.UTC(0,0,0),
                    pointInterval: 60 * 1000,
                    marker: {
                        enabled: false,
                        states: {
                            hover: {
                                enabled: false
                            }
                        }
                    }
                }
            },
            series: data
        });
    });
     $.getJSON(window.location.pathname + 'postsbydayall', function (data) {
        $("#spinnerTwo").hide();
        $("#postsByDayAll").show();
        $('#postsByDayAll').highcharts({
            chart: {
                type: 'area',
                zoomType: 'x'
            },
            title: {
                text: 'Activity in channel over time'
            },
            xAxis: {
                type: 'datetime',
                title: {
                    text: "Date"
                }
            },
            yAxis: {
                title: {
                    text: 'Posts'
                }
            },
            tooltip: {
                formatter: function() {
                    return '<b>'+this.y+'</b> posts on <b>'+Highcharts.dateFormat('%b %d, %Y', this.x)+'</b>'
                }
            },
            legend: {
                enabled: false
            },
            credits: {
                enabled: false
            },
            plotOptions: {
                area: {
                    marker: {
                        enabled: false,
                        symbol: 'circle',
                        radius: 2,
                        states: {
                            hover: {
                                enabled: true
                            }
                        }
                    }
                },
                spline: {
                    marker: {
                        enabled: false,
                        states: {
                            hover: {
                                enabled: false
                            }
                        }
                    }
                }
            },
            series: data
        });
    });
});
