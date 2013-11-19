$(function () {
     $("#spinnerTwo").show();
     $.getJSON('https://sadbox.org/geekhack/postsbydayall', function (data) {
        $("#spinnerTwo").hide();
        $("#postsByDayAll").show();
        Highcharts.setOptions({
            global : {
                useUTC : false
            }
        });
        var currentime = new Date();
        var parsedData = data.data.map{ function(item) { return [Date.UTC(item[0], item[1], item[2]), item[3]]; } }
        $('#postsByDayAll').highcharts({
            chart: {
                type: 'area'
            },
            title: {
                text: 'Activity in channel over time'
            },
            xAxis: {
                type: 'datetime',
                dateTimeLabelFormats: {
                    day: '%H:%M'
                },
                title: {
                    text: "Time of Post (UTC Offset: "+currentime.getTimezoneOffset()/60+")"
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
                    pointStart: parsedData[0][0],
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
                }
            },
            series: parsedData
        });
    });
});
